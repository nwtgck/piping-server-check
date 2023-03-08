package check

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/util"
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
	path := "/" + uuid.NewString()
	bodyString := "my message"
	url := serverUrl + path

	contentType := "text/plain"
	gettingCh := make(chan struct{}, 1)
	// h3 does not support httptrace: https://github.com/quic-go/quic-go/issues/3342
	var getWroteRequestNotForH3 bool
	postRespCh := make(chan *http.Response, 1)
	postFinished := make(chan struct{})
	go func() {
		defer func() { postFinished <- struct{}{} }()
		postReq, err := http.NewRequest(sendMethod, url, strings.NewReader(bodyString))
		if err != nil {
			runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
			return
		}
		ensureContentLengthExits(postReq)
		postReq.Header.Set("Content-Type", contentType)
		postResp, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, runCheckResultCh)
		if !postOk {
			return
		}
		// TODO: handle postResp.Body
		// TODO: not subcheck in HTTP/1.0
		if getWroteRequestNotForH3 {
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver, Warnings: []ResultWarning{NewWarning("sender's response header should be arrived before receiver's request", nil)}}
		} else {
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSenderResponseBeforeReceiver}
			if config.Protocol == ProtocolH3 {
				runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSamePathSenderRejection, Warnings: []ResultWarning{NewWarning("not supported in h3", nil)}}
			} else {
				ctx, cancel := context.WithCancel(context.Background())
				go func() { <-gettingCh; cancel() }()
				checkSenderConnected(ctx, config, sendMethod, url, runCheckResultCh)
			}
		}
		postRespCh <- postResp
	}()

	select {
	case <-postRespCh:
	case <-time.After(config.SenderResponseBeforeReceiverTimeout):
	}

	gettingCh <- struct{}{}
	getReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create GET request", err))
		return
	}
	getReq = getReq.WithContext(httptrace.WithClientTrace(getReq.Context(), &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			getWroteRequestNotForH3 = true
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

	checkTransferForReusePath(config, url, runCheckResultCh)
	return
}

func checkSenderConnected(ctx context.Context, config *Config, sendMethod string, url string, runCheckResultCh chan<- RunCheckResult) {
	sendHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer sendHttpClient.CloseIdleConnections()
	var bodyReader io.Reader
	if config.Protocol == ProtocolHttp1_0 || config.Protocol == ProtocolHttp1_0_tls {
		// HTTP/1.0 does not support chunked encoding
		bodyReader = strings.NewReader("my message")
	} else {
		bodyReader, _ = io.Pipe()
	}
	sendReq, err := http.NewRequest(sendMethod, url, bodyReader)
	if err != nil {
		runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSamePathSenderRejection, Errors: []ResultError{NewError(fmt.Sprintf("failed to create %s request", sendMethod), err)}}
		return
	}
	sendReq = sendReq.WithContext(ctx)
	sendResp, err := sendHttpClient.Do(sendReq)
	if err != nil {
		runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSamePathSenderRejection, Errors: []ResultError{NewError(fmt.Sprintf("failed to %s", sendMethod), err)}}
		return
	}
	if util.IsHttp4xxError(sendResp) {
		runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSamePathSenderRejection}
	} else {
		runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameSamePathSenderRejection, Errors: []ResultError{NewError(fmt.Sprintf("expected 4xx status but found: %d", sendResp.StatusCode), nil)}}
	}
}
