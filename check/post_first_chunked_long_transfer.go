package check

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
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
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			if len(config.SortedTransferSpans) == 0 {
				// skipped
				return
			}
			if slices.Contains([]Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}, config.Protocol) {
				// Skip because HTTP/1.0 has not chunked encoding
				// TODO: create both long-transfer with chunked encoding and long-transfer with Content-Length
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
			var randomSeed int64 = 11
			finishSendReaderCh := make(chan struct{}, 1)
			sendingReader := util.NewFinishableReader(util.NewRateLimitReader(rand.New(rand.NewSource(randomSeed)), config.TransferBytePerSec), finishSendReaderCh)
			expectedReader := rand.New(rand.NewSource(randomSeed))

			postRespOneshot := oneshot.NewOneshot[*http.Response]()
			go func() {
				defer postRespOneshot.Done()
				postReq, err := http.NewRequest("POST", url, sendingReader)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create request", err)))
					return
				}
				postResp, postOk := sendOrGetAndCheck(postHttpClient, postReq, config.Protocol, reporter)
				if !postOk {
					return
				}
				postRespOneshot.Send(postResp)
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
			case getResp = <-getRespOneshot.Channel():
			case <-time.After(config.GetResponseReceivedTimeout):
				reporter.Report(NewRunCheckResultWithOneError(NewError(fmt.Sprintf("failed to get receiver's response in %s", config.GetResponseReceivedTimeout), nil)))
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
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read GET response body", err)))
					return
				}
				totalReadByte += n
				if _, err := io.ReadFull(expectedReader, expectedBuff[0:n]); err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read expected reader", err)))
					return
				}
				if !bytes.Equal(buff[:n], expectedBuff[:n]) {
					reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError("different body", nil)}})
					return
				}
				if time.Since(startTime) >= transferSpan {
					if !readerFinishSent {
						reporter.Report(RunCheckResult{SubCheckName: SubCheckNamePartialTransfer, Message: fmt.Sprintf("%v: %s transferred", transferSpan, util.HumanizeBytes(float64(totalReadByte)))})
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
			if ok := checkCloseReceiverRespBody(getResp, reporter); !ok {
				return
			}
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
