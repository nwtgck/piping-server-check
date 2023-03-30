package check

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"time"
)

func post_first_byte_by_byte_streaming() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			if slices.Contains([]Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}, config.Protocol) {
				// Skip because HTTP/1.0 has not chunked encoding
				return
			}
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, &reporter)
			if !ok {
				return
			}
			defer stopServerIfNeed()

			postHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := newHTTPClient(config.Protocol, config.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := "/" + uuid.NewString()
			url := serverUrl + path

			postRespOneshot := oneshot.NewOneshot[*http.Response]()
			pr, pw := io.Pipe()
			go func() {
				defer postRespOneshot.Done()
				postReq, err := http.NewRequest("POST", url, pr)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create request", err)))
					return
				}
				postResp, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, reporter)
				if !postOk {
					return
				}
				postRespOneshot.Send(postResp)
				// Need to send one byte to GET
				if _, err := pw.Write([]byte{0}); err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to send request body", err)))
					return
				}
			}()

			select {
			case _, ok := <-postRespOneshot.Channel():
				if !ok {
					return
				}
			case <-time.After(config.SenderResponseBeforeReceiverTimeout):
			}

			getRespOneshot := oneshot.NewOneshot[*http.Response]()
			go func() {
				defer getRespOneshot.Done()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create request", err)))
					return
				}
				getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, reporter)
				if !getOk {
					return
				}
				getRespOneshot.Send(getResp)
			}()

			var getResp *http.Response
			select {
			case getResp, ok = <-getRespOneshot.Channel():
				if !ok {
					return
				}
			case <-time.After(config.GetResponseReceivedTimeout):
				reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to get receiver's response in %s", config.GetResponseReceivedTimeout), nil)))
				return
			}

			firstByteChecked := make(chan struct{}, 1)
			go func() {
				var buff [1]byte
				if _, err := io.ReadFull(getResp.Body, buff[:]); err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read GET response body", err)))
					return
				}
				if buff[0] != 0 {
					reporter.Report(NewRunCheckResultWithOneError(NewError("different first byte of body", nil)))
					return
				}
				firstByteChecked <- struct{}{}
			}()

			select {
			case <-firstByteChecked:
			case <-time.After(config.FirstByteCheckTimeout):
				reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to get first byte in %s", config.FirstByteCheckTimeout), nil)))
				return
			}

			var buff [1]byte
			for i := 1; i < 256; i++ {
				writeBytes := []byte{byte(i)}
				if _, err := pw.Write(writeBytes); err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to send request body", err)))
					return
				}
				if _, err := io.ReadFull(getResp.Body, buff[:]); err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read GET response body", err)))
					return
				}
				if byte(i) != buff[0] {
					reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError(fmt.Sprintf("different body: i=%d", i), nil)}})
					return
				}
			}
			if err := pw.Close(); err != nil {
				reporter.Report(RunCheckResult{Errors: []ResultError{NewError("failed to close sending body", err)}})
				return
			}
			n, err := getResp.Body.Read(buff[:])
			if n != 0 {
				reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError(fmt.Sprintf("expected to read 0 bytes but %d", n), err)}})
				return
			}
			if err != io.EOF {
				reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError("expected to get EOF", err)}})
				return
			}
			if ok := checkCloseReceiverRespBody(getResp, reporter); !ok {
				return
			}
			// TODO: POST-timeout (already GET)
			postResp, ok := <-postRespOneshot.Channel()
			if !ok {
				return
			}
			if ok := checkSenderRespReadUp(SubCheckNameTransferred, postResp, reporter); !ok {
				return
			}
			reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred})
			return
		},
	}
}
