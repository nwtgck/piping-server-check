package check

import (
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
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
			getRespOneshot := oneshot.NewOneshot[*http.Response]()
			getReqWroteRequestCh := make(chan struct{})
			go func() {
				defer getRespOneshot.Done()
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
					return
				}
				getRespOneshot.Send(getResp)
			}()

			if config.Protocol == ProtocolH3 {
				// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
				reporter.Report(RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. Ensuring GET-request-first is not supported in HTTP/3", nil)}})
				time.Sleep(config.GetReqWroteRequestWaitForH3)
			} else {
				// Wait for the GET request
				select {
				case <-getReqWroteRequestCh:
				case _, ok := <-getRespOneshot.Channel():
					if !ok {
						return
					}
				}
			}

			postRespOneshot := oneshot.NewOneshot[*http.Response]()
			go func() {
				defer postRespOneshot.Done()
				postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create POST request", err)))
					return
				}
				postReq.Header.Set("Content-Type", contentType)
				postResp, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, reporter)
				if !postOk {
					return
				}
				postRespOneshot.Send(postResp)
			}()

			// TODO: GET-timeout (fixed-length body)
			getResp, ok := <-getRespOneshot.Channel()
			if !ok {
				return
			}
			checkContentTypeForwarding(getResp, contentType, reporter)
			checkXRobotsTag(getResp, reporter)
			bodyBytes, err := io.ReadAll(getResp.Body)
			if err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read up", err)))
				return
			}
			if ok := checkCloseReceiverRespBody(getResp, reporter); !ok {
				return
			}
			if string(bodyBytes) != bodyString {
				reporter.Report(NewRunCheckResultWithOneError(NewError("message different", nil)))
				return
			}

			// TODO: POST-timeout (already GET)
			postResp, ok := <-postRespOneshot.Channel()
			if !ok {
				return
			}
			if ok := checkSenderRespReadUp(postResp, reporter); !ok {
				return
			}

			reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred})

			checkTransferForReusePath(config, url, reporter)
			return
		},
	}
}
