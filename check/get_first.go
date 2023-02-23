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
		run: func(config *Config, subConfig *SubConfig, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			serverUrl, stopServer, resultErrors := prepareServer(config, subConfig)
			if len(resultErrors) != 0 {
				runCheckResultCh <- RunCheckResult{Errors: resultErrors}
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

			getReqWroteRequest := make(chan bool)
			getReqFinished := make(chan struct{})
			go func() {
				defer func() { getReqFinished <- struct{}{} }()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
					return
				}
				clientTrace := &httptrace.ClientTrace{
					WroteRequest: func(info httptrace.WroteRequestInfo) {
						getReqWroteRequest <- true
						close(getReqWroteRequest)
					},
				}
				getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), clientTrace))
				getResp, err := getHttpClient.Do(getReq)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to get", err))
					getReqWroteRequest <- false
					return
				}
				if resultErrors := checkProtocol(getResp, subConfig.Protocol); len(resultErrors) != 0 {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
				}
				if getResp.StatusCode != 200 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(getResp.StatusCode))
					return
				}
				bodyBytes, err := io.ReadAll(getResp.Body)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read up", err))
					return
				}
				if string(bodyBytes) != bodyString {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("message different", nil))
					return
				}
			}()

			// Wait for the GET request
			if ok := <-getReqWroteRequest; !ok {
				return
			}

			postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
				return
			}
			postReq.Header.Set("Content-Type", "text/plain")
			postResp, err := postHttpClient.Do(postReq)
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to post", err))
				return
			}
			if resultErrors := checkProtocol(postResp, subConfig.Protocol); len(resultErrors) != 0 {
				runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
			}
			if postResp.StatusCode != 200 {
				runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(postResp.StatusCode))
				return
			}
			<-getReqFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
