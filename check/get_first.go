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
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, reporter)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			bodyString := "my message"
			url := serverUrl + path

			contentType := "text/plain"
			getReqWroteRequestCh := make(chan struct{})
			getReqFailedCh := make(chan struct{})
			getFinished := make(chan struct{})
			go func() {
				defer func() { getFinished <- struct{}{} }()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create GET request", err)))
					return
				}
				getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), &httptrace.ClientTrace{
					WroteRequest: func(info httptrace.WroteRequestInfo) {
						getReqWroteRequestCh <- struct{}{}
					},
				}))
				getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, reporter)
				if !getOk {
					getReqFailedCh <- struct{}{}
					return
				}
				checkContentTypeForwarding(getResp, contentType, reporter)
				checkXRobotsTag(getResp, reporter)
				bodyBytes, err := io.ReadAll(getResp.Body)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read up", err)))
					return
				}
				if string(bodyBytes) != bodyString {
					reporter.Report(NewRunCheckResultWithOneError(NewError("message different", nil)))
					return
				}
			}()

			if config.Protocol == ProtocolH3 {
				// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
				reporter.Report(RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. Ensuring GET-request-first is not supported in HTTP/3", nil)}})
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
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create POST request", err)))
				return
			}
			postReq.Header.Set("Content-Type", contentType)
			_, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, reporter)
			if !postOk {
				return
			}
			<-getFinished
			reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred})

			checkTransferForReusePath(config, url, reporter)
			return
		},
	}
}
