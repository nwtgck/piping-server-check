package check

import (
	"fmt"
	_ "github.com/k0kubun/pp/v3" // Not used but do not remove. It is useful to create tests
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var pipingServerPkg1_12_8Path string
var goPipingServer0_4_0Path string

func init() {
	var err error
	pipingServerPkg1_12_8Path, err = downloadPipingServerPkgIfNotCached("1.12.8-1")
	if err != nil {
		panic(err)
	}
	goPipingServer0_4_0Path, err = downloadGoPipingServerIfNotCached("0.4.0")
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

func TestRunChecksForHTTP1_1(t *testing.T) {
	checks := AllChecks()
	config := Config{
		RunServerCmd:                        []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT", pipingServerPkg1_12_8Path)},
		Concurrency:                         10,
		SenderResponseBeforeReceiverTimeout: 1 * time.Second,
		FirstByteCheckTimeout:               1 * time.Second,
		GetResponseReceivedTimeout:          1 * time.Second,
	}
	protocols := []Protocol{ProtocolHttp1_1}
	var results []Result
	for result := range RunChecks(checks, &config, protocols) {
		results = append(results, result)
	}
	truePointer := new(bool)
	*truePointer = true
	expected := []Result{
		{Name: "post_first.sender_response_before_receiver", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "get_first.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.sender_response_before_receiver", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.content_type_forwarding", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.x_robots_tag_none", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "put.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
		{Name: "post_first_byte_by_byte_streaming.transferred", Protocol: ProtocolHttp1_1, OkForJson: truePointer},
	}
	assert.Equal(t, expected, results)
}

func TestRunChecksForH2C(t *testing.T) {
	checks := AllChecks()
	config := Config{
		RunServerCmd: []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT", goPipingServer0_4_0Path)},
		Concurrency:  10,
		// Short timeouts are OK because the checks are always timeout when they are long
		SenderResponseBeforeReceiverTimeout: 100 * time.Millisecond,
		FirstByteCheckTimeout:               100 * time.Millisecond,
		GetResponseReceivedTimeout:          100 * time.Millisecond,
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
		"post_first_byte_by_byte_streaming",
	})
	assert.ElementsMatch(t, warningResultNames, []string{
		"post_first.sender_response_before_receiver",
		"put.sender_response_before_receiver",
	})
}

func TestRunChecksForH3(t *testing.T) {
	keyPath, certPath, err := createKeyAndCert()
	if err != nil {
		panic(err)
	}
	checks := AllChecks()
	config := Config{
		RunServerCmd:      []string{"sh", "-c", fmt.Sprintf("exec %s --http-port=$HTTP_PORT --enable-https --https-port=$HTTPS_PORT --key-path=%s --crt-path=%s --enable-http3", goPipingServer0_4_0Path, keyPath, certPath)},
		TlsSkipVerifyCert: true,
		Concurrency:       10,
		// Short timeouts are OK because the checks are always timeout when they are long
		SenderResponseBeforeReceiverTimeout: 100 * time.Millisecond,
		FirstByteCheckTimeout:               100 * time.Millisecond,
		GetResponseReceivedTimeout:          100 * time.Millisecond,
		GetReqWroteRequestWaitForH3:         0,
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
		"post_first_byte_by_byte_streaming",
	})
	assert.ElementsMatch(t, warningResultNames, []string{
		"get_first",
	})
}
