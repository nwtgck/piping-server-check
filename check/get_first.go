package check

import (
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
)

func get_first() Check {
	return Check{
		Name:              checkName(),
		AcceptedProtocols: []Protocol{Http1_0, Http1_1, H2, H2c},
		run: func(config *Config, subConfig *SubConfig) (result Result) {
			serverUrl, stopServer := prepareServer(config, subConfig, &result)
			if len(result.Errors) != 0 {
				return
			}
			defer stopServer()

			postHttpClient := httpProtocolToClient(subConfig.Protocol, subConfig.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := httpProtocolToClient(subConfig.Protocol, subConfig.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := uuid.NewString()
			bodyString := "my message"
			url := serverUrl + "/" + path

			getReqWroteRequest := make(chan struct{})
			getReqFinished := make(chan struct{})
			go func() {
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					result.Errors = append(result.Errors, NewError("failed to create GET request", err))
					return
				}
				clientTrace := &httptrace.ClientTrace{
					WroteRequest: func(info httptrace.WroteRequestInfo) {
						getReqWroteRequest <- struct{}{}
						close(getReqWroteRequest)
					},
				}
				getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), clientTrace))
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
				getReqFinished <- struct{}{}
			}()

			// Wait for the GET request
			<-getReqWroteRequest

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
			<-getReqFinished
			return
		},
	}
}
