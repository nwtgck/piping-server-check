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

			ch := make(chan struct{})

			go func() {
				postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
				if err != nil {
					result.Errors = append(result.Errors, NewError("failed to create POST request", err))
					return
				}
				postResp, err := postHttpClient.Do(postReq)
				if err != nil {
					result.Errors = append(result.Errors, NewError("failed to post", err))
					return
				}
				if postResp.StatusCode != 200 {
					result.Errors = append(result.Errors, NotOkStatusError(postResp.StatusCode))
					return
				}
				ch <- struct{}{}
			}()

			////// Post
			//time.Sleep(1 * time.Second)

			go func() {
				getResp, err := getHttpClient.Get(url)
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
				ch <- struct{}{}
			}()

			<-ch
			<-ch
			return
		},
	}
}
