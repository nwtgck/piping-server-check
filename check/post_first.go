package check

import (
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// TODO: check PUT method

func post_first() Check {
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

			contentType := "text/plain"
			var getWroteRequest bool
			postReqArrived := make(chan struct{})
			postFinished := make(chan struct{})
			go func() {
				defer func() { postFinished <- struct{}{} }()
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
				postReqArrived <- struct{}{}
				if getWroteRequest {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver, Warnings: []ResultWarning{NewWarning("sender's response header should be arrived before receiver's request", nil)}}
				} else {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver}
				}
				if resultErrors := checkProtocol(postResp, subConfig.Protocol); len(resultErrors) != 0 {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
				}
				if postResp.StatusCode != 200 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(postResp.StatusCode))
					return
				}
			}()

			select {
			case <-postReqArrived:
			// TODO: hard code
			case <-time.After(5 * time.Second):
			}

			getTrace := &httptrace.ClientTrace{
				WroteRequest: func(info httptrace.WroteRequestInfo) {
					getWroteRequest = true
				},
			}
			getReq, err := http.NewRequest("GET", url, nil)
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
				return
			}
			getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), getTrace))
			getResp, err := getHttpClient.Do(getReq)
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to get", err))
				return
			}
			if resultErrors := checkProtocol(getResp, subConfig.Protocol); len(resultErrors) != 0 {
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
			<-postFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
