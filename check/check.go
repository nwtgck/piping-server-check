package check

import (
	"fmt"
	"github.com/nwtgck/piping-server-check/http1_0_round_tripper"
	"github.com/nwtgck/piping-server-check/util"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const Http1_0 = "http1.0"
const Http1_1 = "http1.1"
const H2 = "h2"
const H2c = "h2c"

type Config struct {
	// $HTTP_PORT, $HTTPS_PORT
	RunServerCmd []string
}

// TODO: name
type SubConfig struct {
	Protocol string
}

func httpProtocolToClient(protocol string) *http.Client {
	switch protocol {
	//case Http1_0:
	case Http1_1:
		return &http.Client{
			Transport: http1_0_round_tripper.Http10RoundTripper{},
		} // TODO:
	case H2:
		return &http.Client{} // TODO:
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
	Protocol  string        `json:"protocol"`
	OkForJson *bool         `json:"ok,omitempty"`
	Errors    []ResultError `json:"errors,omitempty"`
}

type Check struct {
	Name              string
	AcceptedProtocols []string
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

func prepareHTTPServer(config *Config, result *Result) (httpUrl string, stopSerer func(), err error) {
	httpPort, err := util.GetTCPPort()
	if err != nil {
		result.Errors = append(result.Errors, FailedToGetPortError())
		return
	}

	cmd, _, stderr, err := startServer(config.RunServerCmd, httpPort, "")
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
	httpAddress := net.JoinHostPort("localhost", httpPort)
	httpUrl = "http://" + httpAddress

	go func() {
		waitTCPServer(httpAddress)
		errCh <- nil
	}()

	err = <-errCh
	return
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
