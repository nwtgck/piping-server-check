package check

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/nwtgck/piping-server-check/http10_round_tripper"
	"github.com/nwtgck/piping-server-check/util"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/atomic"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Protocol string

const (
	ProtocolHttp1_0     = Protocol("http1.0")
	ProtocolHttp1_0_tls = Protocol("http1.0-tls")
	ProtocolHttp1_1     = Protocol("http1.1")
	ProtocolHttp1_1_tls = Protocol("http1.1-tls")
	ProtocolH2          = Protocol("h2")
	ProtocolH2c         = Protocol("h2c")
	ProtocolH3          = Protocol("h3")
)

type Config struct {
	RunServerCmd                                     []string
	HealthCheckPath                                  string
	ServerSchemalessUrl                              string
	Protocol                                         Protocol
	TlsSkipVerifyCert                                bool
	Concurrency                                      uint
	SenderResponseBeforeReceiverTimeout              time.Duration
	FirstByteCheckTimeout                            time.Duration
	GetResponseReceivedTimeout                       time.Duration
	GetReqWroteRequestWaitForH3                      time.Duration // because httptrace not supported: https://github.com/quic-go/quic-go/issues/3342
	TransferBytePerSec                               int
	SortedTransferSpans                              []time.Duration
	WaitDurationAfterSenderCancel                    time.Duration
	WaitDurationBetweenReceiverWroteRequestAndCancel time.Duration
	WaitDurationAfterReceiverCancel                  time.Duration
}

func protocolUsesTls(protocol Protocol) bool {
	switch protocol {
	case ProtocolHttp1_0_tls, ProtocolHttp1_1_tls, ProtocolH2, ProtocolH3:
		return true
	default:
		return false
	}
}

func newHTTPClient(protocol Protocol, tlsSkipVerifyCert bool) *http.Client {
	tlsConfig := &tls.Config{InsecureSkipVerify: tlsSkipVerifyCert}
	// TODO: impl
	switch protocol {
	case ProtocolHttp1_0, ProtocolHttp1_0_tls:
		return &http.Client{
			Transport: &http10_round_tripper.Http10RoundTripper{
				TLSClientConfig: tlsConfig,
			},
		}
	case ProtocolHttp1_1, ProtocolHttp1_1_tls:
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	case ProtocolH2c:
		return &http.Client{
			// (base: https://github.com/thrawn01/h2c-golang-example/tree/cafa2960ca5df81100b61a1785b37f5ed749b87d)
			Transport: &http2.Transport{
				AllowHTTP:       true,
				TLSClientConfig: tlsConfig,
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
	case ProtocolH2:
		return &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	case ProtocolH3:
		return &http.Client{
			Transport: &http3.RoundTripper{
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

func ContentTypeMismatchError(expectedContentType string, actualContentType string) ResultError {
	return ResultError{Message: fmt.Sprintf("Content-Type should be %s but found %s", expectedContentType, actualContentType)}
}

type ResultWarning struct {
	Message string `json:"message"`
}

func NewWarning(message string, err error) ResultWarning {
	if err == nil {
		return ResultWarning{Message: message}
	}
	return ResultWarning{Message: fmt.Sprintf("%s: %+v", message, err)}
}

type Result struct {
	// result name can be "<check name>.<subcheck name>" or "<check name>"
	Name      string          `json:"name"`
	Protocol  Protocol        `json:"protocol"`
	Message   string          `json:"message,omitempty"`
	OkForJson *bool           `json:"ok,omitempty"`
	Errors    []ResultError   `json:"errors,omitempty"`
	Warnings  []ResultWarning `json:"warnings,omitempty"`
}

// Subcheck name is top-level. The same subcheck names in different checks should be the same meaning.
// Purpose: Compromise in a narrow area not the entire check.
const (
	SubCheckNameProtocol                     = "protocol"
	SubCheckNameSenderResponseBeforeReceiver = "sender_response_before_receiver"
	SubCheckNameSamePathSenderRejection      = "same_path_sender_rejection"
	SubCheckNameContentTypeForwarding        = "content_type_forwarding"
	SubCheckNameContentDispositionForwarding = "content_disposition_forwarding"
	SubCheckNameXRobotsTagNone               = "x_robots_tag_none"
	SubCheckNameTransferred                  = "transferred"
	SubCheckNameReusePath                    = "reuse_path"
	SubCheckNamePartialTransfer              = "partial_transfer"
)

type RunCheckResult struct {
	// empty string is ok
	SubCheckName string
	Message      string
	Errors       []ResultError
	Warnings     []ResultWarning
}

func NewRunCheckResultWithOneError(resultError ResultError) RunCheckResult {
	return RunCheckResult{Errors: []ResultError{resultError}}
}

type Check struct {
	Name string
	run  func(config *Config, reporter RunCheckReporter)
}

type RunCheckReporter struct {
	ch     chan<- RunCheckResult
	closed *atomic.Bool
}

func NewRunCheckReporter(ch chan<- RunCheckResult) RunCheckReporter {
	return RunCheckReporter{ch: ch, closed: atomic.NewBool(false)}
}

func (r *RunCheckReporter) Report(result RunCheckResult) {
	if r.closed.Load() {
		return
	}
	r.ch <- result
}

func (r *RunCheckReporter) Close() {
	r.closed.Store(true)
	close(r.ch)
}

func getCheckName() string {
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

func waitHTTPServer(httpClient *http.Client, healthCheckUrl string) {
	time.Sleep(100 * time.Millisecond)
	for {
		resp, err := httpClient.Get(healthCheckUrl)
		if err == nil && (200 <= resp.StatusCode && resp.StatusCode < 300) {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// To avoid port already used error
var prepareServerMutex = new(sync.Mutex)

func prepareServer(config *Config) (serverUrl string, stopSerer func(), resultErrors []ResultError) {
	prepareServerMutex.Lock()
	defer prepareServerMutex.Unlock()
	httpPort, err := util.GetTCPPort()
	if err != nil {
		resultErrors = append(resultErrors, FailedToGetPortError())
		return
	}
	httpsPort, err := util.GetTCPAndUDPPort()
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
		stderrStringCh := make(chan string)
		go func() {
			var buf [2048]byte
			// err can be ignored because empty bytes will be an empty string
			n, _ := io.ReadFull(stderr, buf[:])
			stderrString := string(buf[:n])
			stderrStringCh <- stderrString
		}()
		err := cmd.Wait()
		stderrString := <-stderrStringCh
		if err != nil {
			resultErrors = append(resultErrors, NewError(fmt.Sprintf("%+v, stderr: %s", err, stderrString), err))
		}
		finishCh <- struct{}{}
	}()

	stopSerer = func() {
		cmd.Process.Signal(syscall.SIGTERM)
	}
	serverPort := httpPort
	if protocolUsesTls(config.Protocol) {
		serverPort = httpsPort
	}
	httpAddress := net.JoinHostPort("localhost", serverPort)
	if protocolUsesTls(config.Protocol) {
		serverUrl = "https://" + httpAddress
	} else {
		serverUrl = "http://" + httpAddress
	}

	go func() {
		client := newHTTPClient(config.Protocol, true /* always skip verification for health check */)
		defer client.CloseIdleConnections()
		waitHTTPServer(client, serverUrl+config.HealthCheckPath)
		finishCh <- struct{}{}
	}()

	<-finishCh
	return
}

func prepareServerUrl(config *Config, reporter RunCheckReporter) (serverUrl string, ok bool, stopServerIfNeed func()) {
	if config.ServerSchemalessUrl == "" {
		var stopServer func()
		var resultErrors []ResultError
		serverUrl, stopServer, resultErrors = prepareServer(config)
		if len(resultErrors) != 0 {
			reporter.Report(RunCheckResult{Errors: resultErrors})
			return
		}
		return serverUrl, true, stopServer
	}
	if protocolUsesTls(config.Protocol) {
		serverUrl = "https:" + config.ServerSchemalessUrl
	} else {
		serverUrl = "http:" + config.ServerSchemalessUrl
	}
	return serverUrl, true, func() {}
}

// Use this function when Go standard HTTP library automatically attach it.
func ensureContentLengthExits(req *http.Request) {
	if req.ContentLength <= 0 {
		panic(fmt.Errorf("Content-Length not found in %v", req))
	}
}

func runCheck(c *Check, config *Config, resultCh chan<- Result) {
	runCheckResultCh := make(chan RunCheckResult)
	go func() {
		c.run(config, NewRunCheckReporter(runCheckResultCh))
	}()
	for runCheckResult := range runCheckResultCh {
		var result Result
		if runCheckResult.SubCheckName == "" {
			result.Name = c.Name
		} else {
			result.Name = c.Name + "." + runCheckResult.SubCheckName
		}
		result.Message = runCheckResult.Message
		result.Errors = runCheckResult.Errors
		result.Warnings = runCheckResult.Warnings
		result.Protocol = config.Protocol
		if len(result.Errors) == 0 {
			result.OkForJson = new(bool)
			*result.OkForJson = true
		}
		resultCh <- result
	}
}

func RunChecks(checks []Check, commonConfig *Config, protocols []Protocol) <-chan Result {
	if commonConfig.Concurrency < 1 {
		panic("concurrency should be >= 1")
	}
	ch := make(chan Result)
	resultChForRunCheckCh := make(chan (<-chan Result), commonConfig.Concurrency-1)

	go func() {
		for resultChForRunCheck := range resultChForRunCheckCh {
			for result := range resultChForRunCheck {
				ch <- result
			}
		}
		close(ch)
	}()

	go func() {
		for _, c := range checks {
			for _, protocol := range protocols {
				var resultChForRunCheck chan Result
				resultChForRunCheck = make(chan Result, 128 /* subcheck waits if buffer size is less than the number of subchecks */)
				resultChForRunCheckCh <- resultChForRunCheck
				config := *commonConfig
				config.Protocol = protocol
				go func(c Check, config Config) {
					// TODO: timeout for runCheck considering long-time check
					runCheck(&c, &config, resultChForRunCheck)
					close(resultChForRunCheck)
				}(c, config)
			}
		}
		close(resultChForRunCheckCh)
	}()
	return ch
}
