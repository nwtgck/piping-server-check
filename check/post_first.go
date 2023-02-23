package check

import (
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
)

func post_first() Check {
	return Check{
		Name:              checkName(),
		AcceptedProtocols: []string{Http1_0, Http1_1, H2, H2c},
		run: func(config *Config, subConfig *SubConfig) (result Result) {
			httpServerUrl, stopServer, err := prepareHTTPServer(config, &result)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to prepare HTTP server", err))
				return
			}
			defer stopServer()

			postHttpClient := httpProtocolToClient(subConfig.Protocol)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := httpProtocolToClient(subConfig.Protocol)
			defer getHttpClient.CloseIdleConnections()
			path := uuid.NewString()
			bodyString := "my message"
			url := httpServerUrl + "/" + path

			postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to create POST request", err))
				return
			}
			postReq.Header.Set("Content-Type", "text/plain")
			postResp, err := postHttpClient.Do(postReq)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to post", err))
				return
			}
			checkProtocol(&result, postResp, subConfig.Protocol)
			if postResp.StatusCode != 200 {
				result.Errors = append(result.Errors, NotOkStatusError(postResp.StatusCode))
				return
			}

			getReq, err := http.NewRequest("GET", url, nil)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to create GET request", err))
				return
			}
			getResp, err := getHttpClient.Do(getReq)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to get", err))
				return
			}
			checkProtocol(&result, getResp, subConfig.Protocol)
			if getResp.StatusCode != 200 {
				result.Errors = append(result.Errors, NotOkStatusError(getResp.StatusCode))
				return
			}
			bodyBytes, err := io.ReadAll(getResp.Body)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to read up", err))
				return
			}
			if string(bodyBytes) != bodyString {
				result.Errors = append(result.Errors, NewError("message different", nil))
				return
			}
			return
		},
	}
}
