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
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, runCheckResultCh)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := uuid.NewString()
			bodyString := "my message"
			url := serverUrl + "/" + path

			contentType := "text/plain"
			getReqWroteRequestCh := make(chan struct{})
			getReqFailedCh := make(chan struct{})
			getFinished := make(chan struct{})
			go func() {
				defer func() { getFinished <- struct{}{} }()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
					return
				}
				getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), &httptrace.ClientTrace{
					WroteRequest: func(info httptrace.WroteRequestInfo) {
						getReqWroteRequestCh <- struct{}{}
					},
				}))
				getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, runCheckResultCh)
				if !getOk {
					getReqFailedCh <- struct{}{}
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

			if config.Protocol == ProtocolH3 {
				// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
				runCheckResultCh <- RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. Ensuring GET-request-first is not supported in HTTP/3", nil)}}
				time.Sleep(config.GetReqWroteRequestWaitForH3)
			} else {
				// Wait for the GET request
				select {
				case <-getReqWroteRequestCh:
				case <-getReqFailedCh:
					return
				}
			}

			postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
				return
			}
			postReq.Header.Set("Content-Type", contentType)
			_, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, runCheckResultCh)
			if !postOk {
				return
			}
			<-getFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
