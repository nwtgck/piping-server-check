package check

import (
	"context"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"time"
)

func post_cancel_post() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			// TODO: implement for HTTP/1.0
			if config.Protocol == ProtocolHttp1_0 || config.Protocol == ProtocolHttp1_0_tls {
				runCheckResultCh <- RunCheckResult{Warnings: []ResultWarning{NewWarning("Sorry. This check does not support HTTP/1.0 yet", nil)}}
				return
			}
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, runCheckResultCh)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			bodyString := "my message"
			url := serverUrl + path

			contentType := "text/plain"
			{
				postReq1, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
					return
				}
				ctx, cancel := context.WithCancel(context.Background())
				postReq1 = postReq1.WithContext(ctx)
				postReq1.Header.Set("Content-Type", contentType)
				postResp1, postOk := sendOrGetAndCheck(postHttpClient, postReq1, config.Protocol, runCheckResultCh)
				if !postOk {
					return
				}
				// .Close() is need for piping-server HTTP/2
				if err = postResp1.Body.Close(); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to close POST response body", err))
					return
				}
				cancel()
			}

			// Without this, it works in local but not work in GitHub Actions
			time.Sleep(config.WaitDurationAfterCancel)

			{
				postReq2, err := http.NewRequest("POST", url, strings.NewReader(bodyString))
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create POST request", err))
					return
				}
				postReq2.Header.Set("Content-Type", contentType)
				_, postOk := sendOrGetAndCheck(postHttpClient, postReq2, config.Protocol, runCheckResultCh)
				if !postOk {
					return
				}
				// TODO: use postResp and transfer data
				runCheckResultCh <- RunCheckResult{}
			}
			return
		},
	}
}
