package check

import (
	"crypto/tls"
	"fmt"
	"github.com/nwtgck/piping-server-check/util"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Protocol string

const (
	Http1_0     = Protocol("http1.0")
	Http1_0_tls = Protocol("http1.0-tls")
	Http1_1     = Protocol("http1.1")
	Http1_1_tls = Protocol("http1.1-tls")
	H2          = Protocol("h2")
	H2c         = Protocol("h2c")
)

type Config struct {
	// $HTTP_PORT, $HTTPS_PORT
	RunServerCmd []string
}

// TODO: name
type SubConfig struct {
	Protocol          Protocol
	TlsSkipVerifyCert bool
}

func protocolUsesTls(protocol Protocol) bool {
	switch protocol {
	case Http1_0_tls, Http1_1_tls, H2:
		return true
	default:
		return false
	}
}

func httpProtocolToClient(protocol Protocol, tlsSkipVerifyCert bool) *http.Client {
	tlsConfig := &tls.Config{InsecureSkipVerify: tlsSkipVerifyCert}
	// TODO: impl
	switch protocol {
	//case Http1_0, Http1_0_tls:
	case Http1_1, Http1_1_tls:
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	case H2, H2c:
		return &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	}
	return nil
}

type ResultError struct {
	Message string `json:"message"`
}

func NewError(message string, err error) ResultError {
	if err == nil {
		return ResultError{Message: message}
	}
	return ResultError{Message: fmt.Sprintf("%s: %+v", message, err)}
}

func FailedToGetPortError() ResultError {
	return ResultError{Message: "failed to get port"}
}

func FailedToRunServerError(err error) ResultError {
	return ResultError{Message: fmt.Sprintf("failed to run server: %+v", err)}
}

func NotOkStatusError(status int) ResultError {
	return ResultError{Message: fmt.Sprintf("not OK status: %d", status)}
}

type Result struct {
	Name      string        `json:"name"`
	Protocol  Protocol      `json:"protocol"`
	OkForJson *bool         `json:"ok,omitempty"`
	Errors    []ResultError `json:"errors,omitempty"`
}

const (
	SubCheckNameProtocol    = "protocol"
	SubCheckNameTransferred = "transferred"
)

type RunCheckResult struct {
	// empty string is ok
	SubCheckName string
	Errors       []ResultError
	IsWarning    bool
}

func NewRunCheckResultWithOneError(resultError ResultError) RunCheckResult {
	return RunCheckResult{Errors: []ResultError{resultError}}
}

type Check struct {
	Name              string
	AcceptedProtocols []Protocol
	run               func(config *Config, subConfig *SubConfig, runCheckResultCh chan<- RunCheckResult)
}

func checkName() string {
	counter, _, _, success := runtime.Caller(1)
	if !success {
		panic(fmt.Errorf("failed to run runtime.Caller()"))
	}
	functionName := runtime.FuncForPC(counter).Name()
	index := strings.LastIndex(functionName, ".")
	return functionName[index+1:]
}

func startServer(cmd []string, httpPort string, httpsPort string) (c *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser, err error) {
	c = exec.Command(cmd[0], cmd[1:]...)
	c.Env = append(os.Environ(), "HTTP_PORT="+httpPort, "HTTPS_PORT="+httpsPort)
	stdout, err = c.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err = c.StderrPipe()
	if err != nil {
		return
	}
	err = c.Start()
	return
}

func waitTCPServer(address string) {
	for {
		_, err := net.Dial("tcp", address)
		if err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func prepareServer(config *Config, subConfig *SubConfig) (serverUrl string, stopSerer func(), resultErrors []ResultError) {
	httpPort, err := util.GetTCPPort()
	if err != nil {
		resultErrors = append(resultErrors, FailedToGetPortError())
		return
	}
	httpsPort, err := util.GetTCPPort()
	if err != nil {
		resultErrors = append(resultErrors, FailedToGetPortError())
		return
	}

	cmd, _, stderr, err := startServer(config.RunServerCmd, httpPort, httpsPort)
	if err != nil {
		resultErrors = append(resultErrors, FailedToRunServerError(err))
		return
	}

	finishCh := make(chan struct{})
	go func() {
		var stderrString string
		go func() {
			var buf [2048]byte
			n, _ := io.ReadFull(stderr, buf[:])
			stderrString = string(buf[:n])
		}()
		err := cmd.Wait()
		if err != nil {
			resultErrors = append(resultErrors, NewError(fmt.Sprintf("%+v, stderr: %s", err, stderrString), err))
		}
		finishCh <- struct{}{}
	}()

	stopSerer = func() {
		cmd.Process.Signal(os.Interrupt)
	}
	serverPort := httpPort
	if protocolUsesTls(subConfig.Protocol) {
		serverPort = httpsPort
	}
	httpAddress := net.JoinHostPort("localhost", serverPort)
	if protocolUsesTls(subConfig.Protocol) {
		serverUrl = "https://" + httpAddress
	} else {
		serverUrl = "http://" + httpAddress
	}

	go func() {
		waitTCPServer(httpAddress)
		finishCh <- struct{}{}
	}()

	<-finishCh
	return
}

func checkProtocol(resp *http.Response, expectedProto Protocol) []ResultError {
	var resultErrors []ResultError
	var versionOk bool
	switch expectedProto {
	case Http1_0, Http1_0_tls:
		versionOk = resp.Proto == "HTTP/1.0"
	case Http1_1, Http1_1_tls:
		versionOk = resp.Proto == "HTTP/1.1"
	case H2, H2c:
		versionOk = resp.Proto == "HTTP/2.0"
	}
	if !versionOk {
		resultErrors = append(resultErrors, NewError(fmt.Sprintf("expected %s but %s", expectedProto, resp.Proto), nil))
	}
	shouldUseTls := protocolUsesTls(expectedProto)
	if shouldUseTls && resp.TLS == nil {
		resultErrors = append(resultErrors, NewError("should use TLS but not used", nil))
	}
	if !shouldUseTls && resp.TLS != nil {
		resultErrors = append(resultErrors, NewError("should not use TLS but used", nil))
	}
	return resultErrors
}

func AllChecks() []Check {
	return []Check{
		post_first(),
		get_first(),
		post_first_byte_by_byte_streaming(),
	}
}

func RunCheck(c *Check, config *Config, subConfig *SubConfig, resultCh chan<- Result) {
	runCheckResultCh := make(chan RunCheckResult)
	go func() {
		c.run(config, subConfig, runCheckResultCh)
	}()
	for runCheckResult := range runCheckResultCh {
		var result Result
		if runCheckResult.SubCheckName == "" {
			result.Name = c.Name
		} else {
			result.Name = c.Name + "." + runCheckResult.SubCheckName
		}
		result.Errors = runCheckResult.Errors
		// TODO: use IsWarning
		result.Protocol = subConfig.Protocol
		if len(result.Errors) == 0 {
			result.OkForJson = new(bool)
			*result.OkForJson = true
		}
		resultCh <- result
	}
}
