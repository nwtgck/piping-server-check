package check

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
	"net/http"
	"time"
)

func service_worker_registration_rejection() Check {
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
			getHttpClient.Timeout = 1 * time.Second
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			url := serverUrl + path

			getRespOneshot := oneshot.NewOneshot[*http.Response]()
			go func() {
				defer getRespOneshot.Done()
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
					reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("Service Worker registration should be rejected but status code is %d", getResp.StatusCode), nil)))
				}
				getRespOneshot.Send(getResp)
			}()

			select {
			case _, ok := <-getRespOneshot.Channel():
				if !ok {
					return
				}
			case <-time.After(config.ServiceWorkerRejectionTimeout):
			}

			reporter.Report(RunCheckResult{})
			return
		},
	}
}
