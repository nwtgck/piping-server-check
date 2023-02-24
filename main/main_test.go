package main

import (
	"github.com/nwtgck/piping-server-check/check"
	"github.com/stretchr/testify/assert"
	"testing"
)

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
