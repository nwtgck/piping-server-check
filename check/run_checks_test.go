package check

import (
	"fmt"
	_ "github.com/k0kubun/pp/v3" // Not used but do not remove. It is useful to create tests
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var pipingServerPkg1_12_8Path string
var goPipingServer0_5_0Path string

func init() {
	var err error
	pipingServerPkg1_12_8Path, err = downloadPipingServerPkgIfNotCached("1.12.8-1")
	if err != nil {
		panic(err)
	}
	goPipingServer0_5_0Path, err = downloadGoPipingServerIfNotCached("0.5.0")
	if err != nil {
		panic(err)
	}
}

func TestRunServerCommandFailed(t *testing.T) {
	checks := AllChecks()
	config := Config{
		RunServerCmd: []string{"sh", "-c", "echo 'error on purpose' > /dev/stderr && exit 1"},
		Concurrency:  1,
	}
	protocols := []Protocol{ProtocolHttp1_1}
	for result := range RunChecks(checks, &config, protocols) {
		assert.NotNil(t, result.Errors)
		assert.Contains(t, result.Errors[0].Message, "error on purpose")
	}
}

func TestRunChecksForHTTP1_0(t *testing.T) {
	keyPath, certPath, removeKeyAndCert, err := createKeyAndCert()
	if err != nil {
		panic(err)
	}
	defer removeKeyAndCert()
	checks := AllChecks()
	config := Config{
		RunServerCmd:                        []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT --enable-https --https-port=$HTTPS_PORT --key-path=%s --crt-path=%s", pipingServerPkg1_12_8Path, keyPath, certPath)},
		Concurrency:                         10,
		TlsSkipVerifyCert:                   true,
		SenderResponseBeforeReceiverTimeout: 1 * time.Second,
		FirstByteCheckTimeout:               1 * time.Second,
		GetResponseReceivedTimeout:          1 * time.Second,
		WaitDurationAfterSenderCancel:       1 * time.Second,
		WaitDurationAfterReceiverCancel:     1 * time.Second,
	}
	protocols := []Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}
	var errorResultNames []string
	var warningResultNames []string
	var results []Result
	for result := range RunChecks(checks, &config, protocols) {
		results = append(results, result)
		if len(result.Errors) != 0 {
			errorResultNames = append(errorResultNames, result.Name)
		}
		if len(result.Warnings) != 0 {
			warningResultNames = append(warningResultNames, result.Name)
		}
		assert.Contains(t, []Protocol{ProtocolHttp1_0, ProtocolHttp1_0_tls}, result.Protocol)
	}
	assert.ElementsMatch(t, []string{
		"multipart_form_data",
		"multipart_form_data",
	}, errorResultNames)
	assert.Equal(t, []string{
		"post_first.sender_response_before_receiver",
		"post_first.sender_response_before_receiver",
		"put.sender_response_before_receiver",
		"put.sender_response_before_receiver",
		"post_cancel_post",
		"post_cancel_post",
	}, warningResultNames)
}

func TestRunChecksForHTTP1_1(t *testing.T) {
	checks := AllChecks()
	config := Config{
		RunServerCmd:                        []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT", pipingServerPkg1_12_8Path)},
		Concurrency:                         10,
		SenderResponseBeforeReceiverTimeout: 1 * time.Second,
		FirstByteCheckTimeout:               1 * time.Second,
		GetResponseReceivedTimeout:          1 * time.Second,
		TransferBytePerSec:                  1024 * 1024 * 1024 * 1024,
		SortedTransferSpans:                 []time.Duration{10 * time.Millisecond, 1 * time.Second, 2 * time.Second},
		WaitDurationAfterSenderCancel:       1 * time.Second,
		WaitDurationAfterReceiverCancel:     1 * time.Second,
	}
	protocols := []Protocol{ProtocolHttp1_1}
	var results []Result
	for result := range RunChecks(checks, &config, protocols) {
		// Remove messages because some of them are not predictable
		result.Message = ""
		results = append(results, result)
	}
	truePointer := new(bool)
	*truePointer = true
	expected := []Result{
		{Name: "post_first.sender_response_before_receiver", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.same_path_sender_rejection", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.reuse_path", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.reuse_path", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.sender_response_before_receiver", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.same_path_sender_rejection", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.reuse_path", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_cancel_post", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_cancel_get", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "service_worker_registration_rejection", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_byte_by_byte_streaming.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "multipart_form_data.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "multipart_form_data.content_disposition_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "multipart_form_data.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_chunked_long_transfer.partial_transfer", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_chunked_long_transfer.partial_transfer", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_chunked_long_transfer.partial_transfer", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_chunked_long_transfer.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
	}
	assert.Equal(t, expected, results)
}

func TestRunChecksForH2C(t *testing.T) {
	checks := AllChecks()
	config := Config{
		RunServerCmd: []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT", goPipingServer0_5_0Path)},
		Concurrency:  10,
		// Short timeouts are OK because the checks are always timeout when they are long
		SenderResponseBeforeReceiverTimeout: 100 * time.Millisecond,
		FirstByteCheckTimeout:               100 * time.Millisecond,
		GetResponseReceivedTimeout:          100 * time.Millisecond,
		WaitDurationAfterSenderCancel:       1 * time.Second,
		WaitDurationAfterReceiverCancel:     1 * time.Second,
	}
	protocols := []Protocol{ProtocolH2c}
	var errorResultNames []string
	var warningResultNames []string
	for result := range RunChecks(checks, &config, protocols) {
		if len(result.Errors) != 0 {
			errorResultNames = append(errorResultNames, result.Name)
		}
		if len(result.Warnings) != 0 {
			warningResultNames = append(warningResultNames, result.Name)
		}
		assert.Equal(t, ProtocolH2c, result.Protocol)
	}
	assert.ElementsMatch(t, errorResultNames, []string{
		"post_cancel_post",
		"get_cancel_get",
	})
	assert.ElementsMatch(t, warningResultNames, []string{})
}

func TestRunChecksForH3(t *testing.T) {
	keyPath, certPath, removeKeyAndCert, err := createKeyAndCert()
	if err != nil {
		panic(err)
	}
	defer removeKeyAndCert()
	checks := AllChecks()
	config := Config{
		RunServerCmd:      []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT --enable-https --https-port=$HTTPS_PORT --key-path=%s --crt-path=%s --enable-http3", goPipingServer0_5_0Path, keyPath, certPath)},
		TlsSkipVerifyCert: true,
		Concurrency:       10,
		// Short timeouts are OK because the checks are always timeout when they are long
		SenderResponseBeforeReceiverTimeout: 100 * time.Millisecond,
		FirstByteCheckTimeout:               100 * time.Millisecond,
		GetResponseReceivedTimeout:          100 * time.Millisecond,
		GetReqWroteRequestWaitForH3:         0,
		WaitDurationAfterSenderCancel:       1 * time.Second,
		WaitDurationAfterReceiverCancel:     1 * time.Second,
	}
	protocols := []Protocol{ProtocolH3}
	var errorResultNames []string
	var warningResultNames []string
	for result := range RunChecks(checks, &config, protocols) {
		if len(result.Errors) != 0 {
			errorResultNames = append(errorResultNames, result.Name)
		}
		if len(result.Warnings) != 0 {
			warningResultNames = append(warningResultNames, result.Name)
		}
		assert.Equal(t, ProtocolH3, result.Protocol)
	}
	assert.ElementsMatch(t, errorResultNames, []string{
		"post_cancel_post",
	})
	assert.ElementsMatch(t, warningResultNames, []string{
		"get_first",
		"post_first.same_path_sender_rejection",
		"put.same_path_sender_rejection",
		"get_cancel_get",
	})
}
