package check

import (
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"time"
)

func post_first_byte_by_byte_streaming() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
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
			url := serverUrl + "/" + path

			postRespArrived := make(chan struct{}, 1)
			postFinished := make(chan struct{})
			pr, pw := io.Pipe()
			go func() {
				defer func() { postFinished <- struct{}{} }()
				postReq, err := http.NewRequest("POST", url, pr)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create request", err))
					return
				}
				postResp, err := postHttpClient.Do(postReq)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to post", err))
					return
				}
				postRespArrived <- struct{}{}
				if resultErrors := checkProtocol(postResp, config.Protocol); len(resultErrors) != 0 {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
				}
				if postResp.StatusCode != 200 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(postResp.StatusCode))
					return
				}
				// Need to send one byte to GET
				if _, err := pw.Write([]byte{0}); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to send request body", err))
					return
				}
			}()

			select {
			case <-postRespArrived:
			case <-time.After(config.SenderResponseBeforeReceiverTimeout):
			}

			var getResp *http.Response
			getResponseReceived := make(chan struct{})
			getFinished := make(chan struct{})
			go func() {
				defer func() { getFinished <- struct{}{} }()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create request", err))
					return
				}
				getResp, err = getHttpClient.Do(getReq)
				getResponseReceived <- struct{}{}
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to get", err))
					return
				}
				if resultErrors := checkProtocol(getResp, config.Protocol); len(resultErrors) != 0 {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameProtocol, Errors: resultErrors}
				}
				if getResp.StatusCode != 200 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NotOkStatusError(getResp.StatusCode))
					return
				}
			}()

			select {
			case <-getResponseReceived:
			case <-time.After(config.GetResponseReceivedTimeout):
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to get receiver's response in %s", config.GetResponseReceivedTimeout), nil))
				return
			}

			firstByteChecked := make(chan struct{}, 1)
			go func() {
				var buff [1]byte
				if _, err := io.ReadFull(getResp.Body, buff[:]); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read GET response body", err))
					return
				}
				if buff[0] != 0 {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("different first byte of body", nil))
					return
				}
				firstByteChecked <- struct{}{}
			}()

			select {
			case <-firstByteChecked:
			case <-time.After(config.FirstByteCheckTimeout):
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to get first byte in %s", config.FirstByteCheckTimeout), nil))
				return
			}

			var buff [1]byte
			for i := 1; i < 256; i++ {
				writeBytes := []byte{byte(i)}
				if _, err := pw.Write(writeBytes); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to send request body", err))
					return
				}
				if _, err := io.ReadFull(getResp.Body, buff[:]); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read GET response body", err))
					return
				}
				if byte(i) != buff[0] {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError(fmt.Sprintf("different body: i=%d", i), nil)}}
					return
				}
			}
			<-postFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
