package check

import (
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
)

func post_first_byte_by_byte_streaming() Check {
	return Check{
		Name:              checkName(),
		AcceptedProtocols: []Protocol{Http1_1, H2, H2c},
		run: func(config *Config, subConfig *SubConfig) (result Result) {
			serverUrl, stopServer := prepareServer(config, subConfig, &result)
			if len(result.Errors) != 0 {
				return
			}
			defer stopServer()

			postHttpClient := httpProtocolToClient(subConfig.Protocol, subConfig.TlsSkipVerifyCert)
			defer postHttpClient.CloseIdleConnections()
			getHttpClient := httpProtocolToClient(subConfig.Protocol, subConfig.TlsSkipVerifyCert)
			defer getHttpClient.CloseIdleConnections()
			path := uuid.NewString()
			url := serverUrl + "/" + path

			pr, pw := io.Pipe()
			postReq, err := http.NewRequest("POST", url, pr)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to create request", err))
				return
			}
			postResp, err := postHttpClient.Do(postReq)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to post", err))
				return
			}
			checkProtocol(&result, postResp, subConfig.Protocol)
			if postResp.StatusCode != 200 {
				result.Errors = append(result.Errors, NotOkStatusError(postResp.StatusCode))
				return
			}
			// Need to send one byte to GET
			if _, err := pw.Write([]byte{0}); err != nil {
				result.Errors = append(result.Errors, NewError("failed to send request body", err))
				return
			}

			getReq, err := http.NewRequest("GET", url, nil)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to create request", err))
				return
			}
			getResp, err := getHttpClient.Do(getReq)
			if err != nil {
				result.Errors = append(result.Errors, NewError("failed to get", err))
				return
			}
			checkProtocol(&result, getResp, subConfig.Protocol)
			if getResp.StatusCode != 200 {
				result.Errors = append(result.Errors, NotOkStatusError(getResp.StatusCode))
				return
			}

			var buff [1]byte
			if _, err = io.ReadFull(getResp.Body, buff[:]); err != nil {
				result.Errors = append(result.Errors, NewError("failed to read GET response body", err))
				return
			}
			if buff[0] != 0 {
				result.Errors = append(result.Errors, NewError("different first byte of body", nil))
				return
			}

			for i := 1; i < 256; i++ {
				writeBytes := []byte{byte(i)}
				if _, err := pw.Write(writeBytes); err != nil {
					result.Errors = append(result.Errors, NewError("failed to send request body", err))
					return
				}
				if _, err = io.ReadFull(getResp.Body, buff[:]); err != nil {
					result.Errors = append(result.Errors, NewError("failed to read GET response body", err))
					return
				}
				if byte(i) != buff[0] {
					result.Errors = append(result.Errors, NewError(fmt.Sprintf("different body: i=%d", i), nil))
					return
				}
			}
			return
		},
	}
}
