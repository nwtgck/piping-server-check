package check

import (
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"time"
)

func post_first_byte_by_byte_streaming() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			if slices.Contains([]Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}, config.Protocol) {
				// Skip because HTTP/1.0 has not chunked encoding
				return
			}
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
				_, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, runCheckResultCh)
				if !postOk {
					return
				}
				postRespArrived <- struct{}{}
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

			getRespCh := make(chan *http.Response)
			getFinished := make(chan struct{})
			go func() {
				defer func() { getFinished <- struct{}{} }()
				getReq, err := http.NewRequest("GET", url, nil)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create request", err))
					return
				}
				getResp, getOk := sendOrGetAndCheck(getHttpClient, getReq, config.Protocol, runCheckResultCh)
				if !getOk {
					return
				}
				getRespCh <- getResp
			}()

			var getResp *http.Response
			select {
			case getResp = <-getRespCh:
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
