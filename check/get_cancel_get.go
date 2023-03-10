package check

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

func get_cancel_get() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, runCheckResultCh)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			url := serverUrl + path

			canceledCh := make(chan struct{})
			func() {
				getReqWroteRequestCh := make(chan struct{})
				getReq1, err := http.NewRequest("GET", url, nil)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
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
						runCheckResultCh <- RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. WroteRequest detection not supported in HTTP/3", nil)}}
						<-time.After(config.GetReqWroteRequestWaitForH3)
					} else {
						<-getReqWroteRequestCh
					}
					cancel()
					canceledCh <- struct{}{}
				}()
				_, err = getHttpClient.Do(getReq1)
				if errors.Is(err, context.Canceled) {
					return
				}
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to GET", err))
					return
				}
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("expected not to receive a response but GET response received", err))
			}()

			<-canceledCh
			time.Sleep(config.WaitDurationAfterReceiverCancel)

			checkTransferForGetCancelGet(config, url, runCheckResultCh)
			return
		},
	}
}

func checkTransferForGetCancelGet(config *Config, url string, runCheckResultCh chan<- RunCheckResult) {
	getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer getHttpClient.CloseIdleConnections()
	postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer postHttpClient.CloseIdleConnections()

	bodyString := "my message"

	getRespCh := make(chan *http.Response)
	getFailedCh := make(chan struct{})
	go func() {
		getReq, err := http.NewRequest("GET", url, nil)
		if err != nil {
			runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
			getFailedCh <- struct{}{}
			return
		}
		getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, runCheckResultCh)
		if !getOk {
			getFailedCh <- struct{}{}
			return
		}
		getRespCh <- getResp
	}()

	postContext, postCancel := context.WithCancel(context.Background())
	postFinishedCh := make(chan struct{})
	go func() {
		postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
		if err != nil {
			runCheckResultCh <- RunCheckResult{Errors: []ResultError{NewError("failed to create POST request", err)}}
			return
		}
		postReq = postReq.WithContext(postContext)
		_, postOk := sendOrGetAndCheck(getHttpClient, postReq, config.Protocol, runCheckResultCh)
		if !postOk {
			return
		}
		postFinishedCh <- struct{}{}
	}()

	var getResp *http.Response
	select {
	case getResp = <-getRespCh:
	case <-getFailedCh:
		postCancel()
		return
	}

	bodyBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		runCheckResultCh <- RunCheckResult{Errors: []ResultError{NewError("failed to read up", err)}}
		return
	}
	if string(bodyBytes) != bodyString {
		runCheckResultCh <- RunCheckResult{Errors: []ResultError{NewError("message different", nil)}}
		return
	}
	runCheckResultCh <- RunCheckResult{}
	<-postFinishedCh

}
