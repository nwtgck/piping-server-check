package check

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/nwtgck/piping-server-check/oneshot"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

func multipart_form_data() Check {
	return Check{
		Name: getCheckName(),
		run: func(config *Config, reporter RunCheckReporter) {
			defer reporter.Close()
			serverUrl, ok, stopServerIfNeed := prepareServerUrl(config, reporter)
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
			contentBytes := func() []byte {
				var buff [8 * 1024 * 1024]byte
				if _, err := io.ReadFull(rand.New(rand.NewSource(11)), buff[:]); err != nil {
					panic(err)
				}
				return buff[:]
			}()
			multipartHeaderContentType := "application/octet-stream"
			multipartHeaderDisposition := `form-data; name="input_data"`

			bodyBuffer := new(bytes.Buffer)
			multipartWriter := multipart.NewWriter(bodyBuffer)
			multipartHeader := make(textproto.MIMEHeader)
			multipartHeader.Set("Content-Type", multipartHeaderContentType)
			multipartHeader.Set("Content-Disposition", multipartHeaderDisposition)
			contentType := multipartWriter.FormDataContentType()
			part, err := multipartWriter.CreatePart(multipartHeader)
			if err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create part", err)))
				return
			}
			if _, err = io.Copy(part, bytes.NewReader(contentBytes)); err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to write content to part", err)))
				return
			}
			if err = multipartWriter.Close(); err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to close part", err)))
				return
			}

			postRespOneshot := oneshot.NewOneshot[*http.Response]()
			go func() {
				defer postRespOneshot.Done()
				postReq, err := http.NewRequest("POST", url, bodyBuffer)
				if err != nil {
					reporter.Report(NewRunCheckResultWithOneError(NewError("failed to create request", err)))
					return
				}
				ensureContentLengthExits(postReq)
				postReq.Header.Set("Content-Type", contentType)
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

			checkContentTypeForwarding(getResp, multipartHeaderContentType, reporter)
			checkContentDispositionForwarding(getResp, multipartHeaderDisposition, reporter)

			getBodyBytes, err := io.ReadAll(getResp.Body)
			if err != nil {
				reporter.Report(NewRunCheckResultWithOneError(NewError("failed to read GET body", err)))
				return
			}
			if !bytes.Equal(getBodyBytes, contentBytes) {
				reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError("different body", nil)}})
				return
			}
			postResp, ok := <-postRespOneshot.Channel()
			if !ok {
				return
			}
			if ok := checkSenderRespReadUp(postResp, reporter); !ok {
				return
			}
			reporter.Report(RunCheckResult{SubCheckName: SubCheckNameTransferred})
			return
		},
	}
}
