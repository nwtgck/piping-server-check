package check

import (
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

func get_first() Check {
	return Check{
		Name:              checkName(),
		AcceptedProtocols: []Protocol{Http1_0, Http1_1, H2, H2c},
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, runCheckResultCh)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			postHttpClient := httpProtocolToClient(config.Protocol, config.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := httpProtocolToClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := uuid.NewString()
			bodyString := "my message"
			url := serverUrl + "/" + path

			contentType := "text/plain"
			getReqWroteRequest := make(chan bool)
			getFinished := make(chan struct{})
			go func() {
				defer func() { getFinished <- struct{}{} }()
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
				if resultErrors := checkProtocol(getResp, config.Protocol); len(resultErrors) != 0 {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
				}
				if getResp.StatusCode != 200 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(getResp.StatusCode))
					return
				}
				receivedContentType := getResp.Header.Get("Content-Type")
				if receivedContentType == contentType {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameContentTypeForwarding}
				} else {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameContentTypeForwarding, Errors: []ResultError{ContentTypeMismatchError(contentType, receivedContentType)}}
				}
				receivedXRobotsTag := getResp.Header.Get("X-Robots-Tag")
				if receivedXRobotsTag == "none" {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameXRobotsTagNone}
				} else {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameXRobotsTagNone, Warnings: []ResultWarning{XRobotsTagNoneWarning(receivedXRobotsTag)}}
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

			if config.Protocol == H3 {
				// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
				runCheckResultCh <- RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. Ensuring GET-request-first is not supported in HTTP/3", nil)}}
				time.Sleep(config.GetReqWroteRequestWaitForH3)
			} else {
				// Wait for the GET request
				if ok := <-getReqWroteRequest; !ok {
					return
				}
			}

			postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
				return
			}
			postReq.Header.Set("Content-Type", contentType)
			postResp, err := postHttpClient.Do(postReq)
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to post", err))
				return
			}
			if resultErrors := checkProtocol(postResp, config.Protocol); len(resultErrors) != 0 {
				runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
			}
			if postResp.StatusCode != 200 {
				runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(postResp.StatusCode))
				return
			}
			<-getFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
