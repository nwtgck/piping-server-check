package check

import (
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

func post_first() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			sendFirstRun("POST", config, runCheckResultCh)
		},
	}
}

func put() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			sendFirstRun("PUT", config, runCheckResultCh)
		},
	}
}

func sendFirstRun(sendMethod string, config *Config, runCheckResultCh chan<- RunCheckResult) {
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
	var getWroteRequest bool
	postRespCh := make(chan *http.Response, 1)
	postFinished := make(chan struct{})
	go func() {
		defer func() { postFinished <- struct{}{} }()
		postReq, err := http.NewRequest(sendMethod, url, strings.NewReader(bodyString))
		if err != nil {
			runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
			return
		}
		postReq.Header.Set("Content-Type", contentType)
		postResp, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, runCheckResultCh)
		if !postOk {
			return
		}
		// TODO: not subcheck in HTTP/1.0
		if getWroteRequest {
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver, Warnings: []ResultWarning{NewWarning("sender's response header should be arrived before receiver's request", nil)}}
		} else {
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver}
		}
		postRespCh <- postResp
	}()

	select {
	case <-postRespCh:
	case <-time.After(config.SenderResponseBeforeReceiverTimeout):
	}

	getReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
		return
	}
	getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			getWroteRequest = true
		},
	}))
	getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, runCheckResultCh)
	if !getOk {
		return
	}
	checkContentTypeForwarding(getResp, contentType, runCheckResultCh)
	checkXRobotsTag(getResp, runCheckResultCh)
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
}
