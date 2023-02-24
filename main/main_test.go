package main

import (
	"fmt"
	"github.com/nwtgck/piping-server-check/check"
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
	checks := check.AllChecks()
	config := check.Config{
		RunServerCmd: []string{"sh", "-c", "echo 'error on purpose' > /dev/stderr && exit 1"},
	}
	protocols := []check.Protocol{check.Http1_1}
	for result := range runChecks(checks, &config, protocols) {
		assert.NotNil(t, result.Errors)
		assert.Contains(t, result.Errors[0].Message, "error on purpose")
	}
}

func TestRunChecksForHTTP1_1(t *testing.T) {
	checks := check.AllChecks()
	config := check.Config{
		RunServerCmd:                        []string{"sh", "-c", fmt.Sprintf("%s --http-port=$HTTP_PORT", pipingServerPkg1_12_8Path)},
		SenderResponseBeforeReceiverTimeout: 1 * time.Second,
		FirstByteCheckTimeout:               1 * time.Second,
		GetResponseReceivedTimeout:          1 * time.Second,
	}
	protocols := []check.Protocol{check.Http1_1}
	var results []check.Result
	for result := range runChecks(checks, &config, protocols) {
		results = append(results, result)
	}
	truePointer := new(bool)
	*truePointer = true
	expected := []check.Result{
		{Name: "post_first.sender_response_before_receiver", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "post_first.content_type_forwarding", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "post_first.x_robots_tag_none", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "post_first.transferred", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "get_first.content_type_forwarding", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "get_first.x_robots_tag_none", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "get_first.transferred", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "put.sender_response_before_receiver", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "put.content_type_forwarding", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "put.x_robots_tag_none", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "put.transferred", Protocol: check.Http1_1, OkForJson: truePointer},
		{Name: "post_first_byte_by_byte_streaming.transferred", Protocol: check.Http1_1, OkForJson: truePointer},
	}
	assert.Equal(t, expected, results)
}

func TestRunChecksForH2C(t *testing.T) {
	checks := check.AllChecks()
	config := check.Config{
		RunServerCmd: []string{"sh", "-c", fmt.Sprintf("%s --http-port=$HTTP_PORT", goPipingServer0_4_0Path)},
		// Short timeouts are OK because the checks are always timeout when they are long
		SenderResponseBeforeReceiverTimeout: 100 * time.Millisecond,
		FirstByteCheckTimeout:               100 * time.Millisecond,
		GetResponseReceivedTimeout:          100 * time.Millisecond,
	}
	protocols := []check.Protocol{check.H2c}
	var errorResultNames []string
	var warningCheckNames []string
	for result := range runChecks(checks, &config, protocols) {
		if len(result.Errors) != 0 {
			errorResultNames = append(errorResultNames, result.Name)
		}
		if len(result.Warnings) != 0 {
			warningCheckNames = append(warningCheckNames, result.Name)
		}
	}
	assert.ElementsMatch(t, errorResultNames, []string{
		"post_first_byte_by_byte_streaming",
	})
	assert.ElementsMatch(t, warningCheckNames, []string{
		"post_first.sender_response_before_receiver",
		"put.sender_response_before_receiver",
	})
}
