package check

import (
	"github.com/google/uuid"
	"io"
	"strings"
)

func post_first() Check {
	name := checkName()
	return Check{
		Name:              name,
		AcceptedProtocols: []string{Http1_0, Http1_1, H2},
		run: func(config *Config, subConfig *SubConfig) (result Result) {
			httpServerUrl, stopServer, err := prepareHTTPServer(config, &result)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to prepare HTTP server", err))
				return
			}
			defer stopServer()

			httpClient := httpProtocolToClient(subConfig.Protocol)
			path := uuid.NewString()
			bodyString := "my message"

			postResp, err := httpClient.Post(httpServerUrl+"/"+path, "text/plain", strings.NewReader(bodyString))
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to post", err))
				return
			}
			if postResp.StatusCode != 200 {
				result.Errors = append(result.Errors, NotOkStatusError(postResp.StatusCode))
				return
			}

			getResp, err := httpClient.Get(httpServerUrl + "/" + path)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to get", err))
				return
			}
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
