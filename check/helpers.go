package check

import (
	"fmt"
	"github.com/nwtgck/piping-server-check/oneshot"
	"io"
	"net/http"
	"strings"
)

func checkProtocol(resp *http.Response, expectedProto Protocol) []ResultError {
	var resultErrors []ResultError
	var versionOk bool
	switch expectedProto {
	case ProtocolHttp1_0, ProtocolHttp1_0_tls:
		versionOk = resp.Proto == "HTTP/1.0"
	case ProtocolHttp1_1, ProtocolHttp1_1_tls:
		versionOk = resp.Proto == "HTTP/1.1"
	case ProtocolH2, ProtocolH2c:
		versionOk = resp.Proto == "HTTP/2.0"
	case ProtocolH3:
		versionOk = resp.Proto == "HTTP/3.0"
	}
	if !versionOk {
		resultErrors = append(resultErrors, NewError(fmt.Sprintf("expected %s but %s", expectedProto, resp.Proto), nil))
	}
	shouldUseTls := protocolUsesTls(expectedProto)
	if shouldUseTls && resp.TLS == nil {
		resultErrors = append(resultErrors, NewError("should use TLS but not used", nil))
	}
	if !shouldUseTls && resp.TLS != nil {
		resultErrors = append(resultErrors, NewError("should not use TLS but used", nil))
	}
	return resultErrors
}

func sendOrGetAndCheck(httpClient *http.Client, req *http.Request, protocol Protocol, reporter RunCheckReporter) (*http.Response, bool) {
	resp, err := httpClient.Do(req)
	if err != nil {
		reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to %s", req.Method), err)))
		return nil, false
	}
	if resultErrors := checkProtocol(resp, protocol); len(resultErrors) != 0 {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors})
	}
	if resp.StatusCode != 200 {
		reporter.Report(NewRunCheckResultWithOneError(NotOkStatusError(resp.StatusCode)))
		return nil, false
	}
	return resp, true
}

func checkContentTypeForwarding(getResp *http.Response, expectedContentType string, reporter RunCheckReporter) {
	receivedContentType := getResp.Header.Get("Content-Type")
	if receivedContentType == expectedContentType {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameContentTypeForwarding})
	} else {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameContentTypeForwarding, Errors: []ResultError{ContentTypeMismatchError(expectedContentType, receivedContentType)}})
	}
}

func checkContentDispositionForwarding(getResp *http.Response, expectedContentDisposition string, reporter RunCheckReporter) {
	receivedContentDisposition := getResp.Header.Get("Content-Disposition")
	if receivedContentDisposition == expectedContentDisposition {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameContentDispositionForwarding})
	} else {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameContentDispositionForwarding, Errors: []ResultError{ContentTypeMismatchError(expectedContentDisposition, receivedContentDisposition)}})
	}
}

func checkXRobotsTag(getResp *http.Response, reporter RunCheckReporter) {
	receivedXRobotsTag := getResp.Header.Get("X-Robots-Tag")
	if receivedXRobotsTag == "none" {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameXRobotsTagNone})
	} else {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameXRobotsTagNone, Warnings: []ResultWarning{XRobotsTagNoneWarning(receivedXRobotsTag)}})
	}
}

func checkTransferForReusePath(config *Config, url string, reporter RunCheckReporter) {
	getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer getHttpClient.CloseIdleConnections()
	postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer postHttpClient.CloseIdleConnections()

	bodyString := "message for reuse"

	getRespOneshot := oneshot.NewOneshot[*http.Response]()
	go func() {
		defer getRespOneshot.Done()
		getReq, err := http.NewRequest("GET", url, nil)
		if err != nil {
			reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create GET request", err)))
			return
		}
		getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, reporter)
		if !getOk {
			return
		}
		getRespOneshot.Send(getResp)
	}()

	postRespOneshot := oneshot.NewOneshot[*http.Response]()
	go func() {
		defer postRespOneshot.Done()
		postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
		if err != nil {
			reporter.Report(RunCheckResult{SubCheckName: SubCheckNameReusePath, Errors: []ResultError{NewError("failed to create POST request", err)}})
			return
		}
		postResp, postOk := sendOrGetAndCheck(getHttpClient, postReq, config.Protocol, reporter)
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
	bodyBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameReusePath, Errors: []ResultError{NewError("failed to read up", err)}})
		return
	}
	if string(bodyBytes) != bodyString {
		reporter.Report(RunCheckResult{SubCheckName: SubCheckNameReusePath, Errors: []ResultError{NewError("message different", nil)}})
		return
	}
	// TODO: POST-timeout (already GET)
	_, ok = <-postRespOneshot.Channel()
	if !ok {
		return
	}
	reporter.Report(RunCheckResult{SubCheckName: SubCheckNameReusePath})
}
