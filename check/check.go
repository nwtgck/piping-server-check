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

type Check struct {
	Name              string
	AcceptedProtocols []Protocol
	run               func(config *Config, subConfig *SubConfig) Result
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

func prepareServer(config *Config, subConfig *SubConfig, result *Result) (serverUrl string, stopSerer func(), err error) {
	httpPort, err := util.GetTCPPort()
	if err != nil {
		result.Errors = append(result.Errors, FailedToGetPortError())
		return
	}
	httpsPort, err := util.GetTCPPort()
	if err != nil {
		result.Errors = append(result.Errors, FailedToGetPortError())
		return
	}

	cmd, _, stderr, err := startServer(config.RunServerCmd, httpPort, httpsPort)
	if err != nil {
		result.Errors = append(result.Errors, FailedToRunServerError(err))
		return
	}

	errCh := make(chan error)
	go func() {
		var stderrString string
		go func() {
			var buf [2048]byte
			n, _ := io.ReadFull(stderr, buf[:])
			stderrString = string(buf[:n])
		}()
		err := cmd.Wait()
		if err == nil {
			errCh <- nil
			return
		}
		errCh <- fmt.Errorf("%+v, stderr: %s", err, stderrString)
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
		errCh <- nil
	}()

	err = <-errCh
	return
}

func checkProtocol(result *Result, resp *http.Response, expectedProto Protocol) {
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
		result.Errors = append(result.Errors, NewError(fmt.Sprintf("expected %s but %s", expectedProto, resp.Proto), nil))
	}
	shouldUseTls := protocolUsesTls(expectedProto)
	if shouldUseTls && resp.TLS == nil {
		result.Errors = append(result.Errors, NewError("should use TLS but not used", nil))
	}
	if !shouldUseTls && resp.TLS != nil {
		result.Errors = append(result.Errors, NewError("should not use TLS but used", nil))
	}
}

func AllChecks() []Check {
	return []Check{
		post_first(),
		get_first(),
		post_first_byte_by_byte_streaming(),
	}
}

func RunCheck(c *Check, config *Config, subConfig *SubConfig) Result {
	result := c.run(config, subConfig)
	result.Name = c.Name
	result.Protocol = subConfig.Protocol
	if len(result.Errors) == 0 {
		result.OkForJson = new(bool)
		*result.OkForJson = true
	}
	return result
}
