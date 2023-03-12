package check

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
	"io"
	"net/http"
	"strings"
)

func simultaneous_request() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()

			successCount := 0
			for i := 0; i < config.NSimultaneousRequests; i++ {
				if ok := transferForSimultaneousRequest(config, reporter); ok {
					successCount++
				}
			}
			if successCount == config.NSimultaneousRequests {
				reporter.Report(RunCheckResult{Message: fmt.Sprintf("all %d simultaneous requests successfully transferred", config.NSimultaneousRequests)})
			} else {
				reporter.Report(RunCheckResult{Errors: []ResultError{NewError(fmt.Sprintf("%d/%d successfully transferred", successCount, config.NSimultaneousRequests), nil)}})
			}
			return
		},
	}
}

func transferForSimultaneousRequest(config *Config, reporter RunCheckReporter) bool {
	serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, reporter)
	if !ok {
		return false
	}
	defer stopServerIfNeed()
	getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer getHttpClient.CloseIdleConnections()
	postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
	defer postHttpClient.CloseIdleConnections()

	url := serverUrl + "/" + uuid.NewString()
	bodyString := "my message"

	getRespOneshot := oneshot.NewOneshot[*http.Response]()
	getReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create GET request", err)))
		return false
	}
	postRespOneshot := oneshot.NewOneshot[*http.Response]()
	postReq, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
	if err != nil {
		reporter.Report(RunCheckResult{Errors: []ResultError{NewError("failed to create POST request", err)}})
		return false
	}

	go func() {
		defer getRespOneshot.Done()
		getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, reporter)
		if !getOk {
			return
		}
		getRespOneshot.Send(getResp)
	}()
	go func() {
		defer postRespOneshot.Done()
		postResp, postOk := sendOrGetAndCheck(getHttpClient, postReq, config.Protocol, reporter)
		if !postOk {
			return
		}
		postRespOneshot.Send(postResp)
	}()

	select {
	case _, ok := <-getRespOneshot.Channel():
		if !ok {
			return false
		}
	case _, ok := <-postRespOneshot.Channel():
		if !ok {
			return false
		}
	}
	getResp, ok := respWithTimeout("", "GET", getRespOneshot, config.FixedLengthBodyGetTimeout, reporter)
	if !ok {
		return false
	}
	bodyBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		reporter.Report(RunCheckResult{Errors: []ResultError{NewError("failed to read up", err)}})
		return false
	}
	if ok := checkCloseReceiverRespBody(getResp, reporter); !ok {
		return false
	}
	if string(bodyBytes) != bodyString {
		reporter.Report(RunCheckResult{Errors: []ResultError{NewError("message different", nil)}})
		return false
	}
	// TODO: POST-timeout (already GET)
	postResp, ok := <-postRespOneshot.Channel()
	if !ok {
		return false
	}
	if ok := checkSenderRespReadUp("", postResp, reporter); !ok {
		return false
	}
	return true
}
