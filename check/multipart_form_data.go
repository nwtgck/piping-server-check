package check

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
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
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create part", err))
				return
			}
			if _, err = io.Copy(part, bytes.NewReader(contentBytes)); err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to write content to part", err))
				return
			}
			if err = multipartWriter.Close(); err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to close part", err))
				return
			}

			postRespArrived := make(chan struct{}, 1)
			postFinished := make(chan struct{})
			go func() {
				defer func() { postFinished <- struct{}{} }()
				postReq, err := http.NewRequest("POST", url, bodyBuffer)
				if err != nil {
					runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to create request", err))
					return
				}
				postReq.Header.Set("Content-Type", contentType)
				// TODO: ensure content-length
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

			checkContentTypeForwarding(getResp, multipartHeaderContentType, runCheckResultCh)
			checkContentDispositionForwarding(getResp, multipartHeaderDisposition, runCheckResultCh)

			getBodyBytes, err := io.ReadAll(getResp.Body)
			if err != nil {
				runCheckResultCh <- NewRunCheckResultWithOneError(NewError("failed to read GET body", err))
				return
			}
			if !bytes.Equal(getBodyBytes, contentBytes) {
				runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred, Errors: []ResultError{NewError("different body", nil)}}
				return
			}
			<-postFinished
			runCheckResultCh <- RunCheckResult{SubCheckName: SubCheckNameTransferred}
			return
		},
	}
}
