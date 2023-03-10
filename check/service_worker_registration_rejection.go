package check

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
)

func service_worker_registration_rejection() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, reporter)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			url := serverUrl + path

			getReq, err := http.NewRequest("GET", url, nil)
			if err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create GET request", err)))
				return
			}
			getReq.Header.Set("Service-Worker", "script")
			getResp, err := getHttpClient.Do(getReq)
			if err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to GET", err)))
				return
			}
			if resultErrors := checkProtocol(getResp, config.Protocol); len(resultErrors) != 0 {
				reporter.Report(RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors})
			}
			if !(400 <= getResp.StatusCode && getResp.StatusCode < 500) {
				reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("Service Worker registration should be rejected but status code is %d", getResp.StatusCode), err)))
			}
			reporter.Report(RunCheckResult{})
			return
		},
	}
}
