package check

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

func get_cancel_get() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, &reporter)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			url := serverUrl + path

			getFailed := false
			canceledCh := make(chan struct{})
			func() {
				getReqWroteRequestCh := make(chan struct{})
				getReq1, err := http.NewRequest("GET", url, nil)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create GET request", err)))
					getFailed = true
					return
				}
				ctx, cancel := context.WithCancel(context.Background())
				getReq1 = getReq1.WithContext(ctx)
				getReq1 = getReq1.WithContext(httptrace.WithClientTrace(getReq1.Context(), &httptrace.ClientTrace{
					WroteRequest: func(info httptrace.WroteRequestInfo) {
						getReqWroteRequestCh <- struct{}{}
					},
				}))
				go func() {
					if config.Protocol == ProtocolH3 {
						// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
						reporter.Report(RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. WroteRequest detection not supported in HTTP/3", nil)}})
						<-time.After(config.GetReqWroteRequestWaitForH3)
					} else {
						<-getReqWroteRequestCh
					}
					// Difficult to detect whether server handles GET request
					time.Sleep(config.WaitDurationBetweenReceiverWroteRequestAndCancel)
					cancel()
					canceledCh <- struct{}{}
				}()
				_, err = getHttpClient.Do(getReq1)
				if errors.Is(err, context.Canceled) {
					return
				}
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to GET", err)))
					getFailed = true
					return
				}
				reporter.Report(NewRunCheckResultWithOneError(NewError("expected not to receive a response but GET response received", err)))
			}()

			if getFailed {
				return
			}

			<-canceledCh
			time.Sleep(config.WaitDurationAfterReceiverCancel)

			checkTransferForGetCancelGet(config, url, reporter)
			return
		},
	}
}

func checkTransferForGetCancelGet(config *Config, url string, reporter RunCheckReporter) {
	getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer getHttpClient.CloseIdleConnections()
	postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer postHttpClient.CloseIdleConnections()

	bodyString := "my message"

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

	postRespOneshot := oneshot.NewOneshot[*http.Response]()
	go func() {
		defer postRespOneshot.Done()
		if config.Protocol == ProtocolH3 {
			// httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
			reporter.Report(RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. WroteRequest detection not supported in HTTP/3", nil)}})
			<-time.After(config.GetReqWroteRequestWaitForH3)
		} else {
			<-getReqWroteRequestCh
		}
		postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
		if err != nil {
			reporter.Report(RunCheckResult{Errors: []ResultError{NewError("failed to create POST request", err)}})
			return
		}
		postResp, postOk := sendOrGetAndCheck(getHttpClient, postReq, config.Protocol, reporter)
		if !postOk {
			return
		}
		postRespOneshot.Send(postResp)
	}()

	select {
	case _, ok := <-getRespOneshot.Channel():
		if !ok {
			return
		}
	case _, ok := <-postRespOneshot.Channel():
		if !ok {
			return
		}
	}

	getResp, ok := <-getRespOneshot.Channel()
	if !ok {
		return
	}
	bodyBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		reporter.Report(RunCheckResult{Errors: []ResultError{NewError("failed to read up", err)}})
		return
	}
	if ok := checkCloseReceiverRespBody(getResp, reporter); !ok {
		return
	}
	if string(bodyBytes) != bodyString {
		reporter.Report(RunCheckResult{Errors: []ResultError{NewError("message different", nil)}})
		return
	}
	postResp, ok := <-postRespOneshot.Channel()
	if !ok {
		return
	}
	if ok := checkSenderRespReadUp("", postResp, reporter); !ok {
		return
	}
	reporter.Report(RunCheckResult{})
}
