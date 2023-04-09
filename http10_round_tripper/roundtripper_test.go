package http10_round_tripper

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func runServer1() (port string, close func()) {
	server := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "server message")
		}),
	}
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	_, port, _ = net.SplitHostPort(listener.Addr().String())
	go func() {
		if err := server.Serve(listener); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			panic(err)
		}
	}()
	close = func() {
		if err := server.Close(); err != nil {
			panic(err)
		}
	}
	return
}

func TestPost(t *testing.T) {
	port, closeServer := runServer1()
	defer closeServer()
	client := &http.Client{
		Transport: &Http10RoundTripper{},
	}
	resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%s", port), "application/octet-stream", strings.NewReader("my body"))
	assert.NoError(t, err)
	assert.Equal(t, "HTTP/1.0", resp.Proto)
	bodyBytes, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "server message", string(bodyBytes))
}
