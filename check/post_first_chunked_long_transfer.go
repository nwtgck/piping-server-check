package check

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/util"
	"golang.org/x/exp/slices"
	"io"
	"math/rand"
	"net/http"
	"time"
)

func post_first_chunked_long_transfer() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, runCheckResultCh chan<- RunCheckResult) {
			defer close(runCheckResultCh)
			if len(config.SortedTransferSpans) == 0 {
				// skipped
				return
			}
			if slices.Contains([]Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}, config.Protocol) {
				// Skip because HTTP/1.0 has not chunked encoding
				// TODO: create both long-transfer with chunked encoding and long-transfer with Content-Length
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
			path := "/" + uuid.NewString()
			url := serverUrl + path
			var randomSeed int64 = 11
			finishSendReaderCh := make(chan struct{}, 1)
			sendingReader := util.NewFinishableReader(util.NewRateLimitReader(rand.New(rand.NewSource(randomSeed)), config.TransferBytePerSec), finishSendReaderCh)
			expectedReader := rand.New(rand.NewSource(randomSeed))

			postRespArrived := make(chan struct{}, 1)
			postFinished := make(chan struct{})
			go func() {
				defer func() { postFinished <- struct{}{} }()
				postReq, err := http.NewRequest("POST", url, sendingReader)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create request", err))
					return
				}
				_, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, runCheckResultCh)
				if !postOk {
					return
				}
				postRespArrived <- struct{}{}
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

			startTime := time.Now()
			totalReadByte := 0
			var buff [1 << 15]byte
			var expectedBuff [1 << 15]byte
			readerFinishSent := false
			for i := 0; ; {
				transferSpan := config.SortedTransferSpans[i]
				n, err := getResp.Body.Read(buff[:])
				if err == io.EOF {
					break
				}
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read GET response body", err))
					return
				}
				totalReadByte += n
				if _, err := io.ReadFull(expectedReader, expectedBuff[0:n]); err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read expected reader", err))
					return
				}
				if !bytes.Equal(buff[:n], expectedBuff[:n]) {
					runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError("different body", nil)}}
					return
				}
				if time.Since(startTime) >= transferSpan {
					if !readerFinishSent {
						runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNamePartialTransfer, Message: fmt.Sprintf("%v: %s transferred", transferSpan, util.HumanizeBytes(float64(totalReadByte)))}
					}
					if i == len(config.SortedTransferSpans)-1 && !readerFinishSent {
						finishSendReaderCh <- struct{}{}
						readerFinishSent = true
					}
					if i != len(config.SortedTransferSpans)-1 {
						i++
					}
				}
			}
			<-postFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
