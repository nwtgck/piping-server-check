package http10_round_tripper

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
)

// TODO: not all httptrace supported
type Http10RoundTripper struct {
	TLSClientConfig *tls.Config
}

func (rt Http10RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn net.Conn
	var err error
	var tlsConnectionState *tls.ConnectionState
	if req.URL.Scheme == "https" {
		d := tls.Dialer{Config: rt.TLSClientConfig}
		conn, err = d.DialContext(req.Context(), "tcp", req.URL.Hostname()+":"+req.URL.Port())
		// TODO: implement
		tlsConnectionState = &tls.ConnectionState{
			HandshakeComplete: true,
			ServerName:        req.Host,
		}
		req.TLS = tlsConnectionState
	} else {
		var d net.Dialer
		conn, err = d.DialContext(req.Context(), "tcp", req.URL.Hostname()+":"+req.URL.Port())
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err = fmt.Fprintf(conn, "%s %s HTTP/1.0\r\n", req.Method, req.URL.RequestURI()); err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintf(conn, "Host: %s\r\n", req.URL.Host); err != nil {
		return nil, err
	}
	if req.ContentLength != 0 {
		if _, err = fmt.Fprintf(conn, "Content-Length: %d\r\n", req.ContentLength); err != nil {
			return nil, err
		}
	}
	for key, values := range req.Header {
		for _, value := range values {
			if _, err = fmt.Fprintf(conn, "%s: %s\r\n", key, value); err != nil {
				return nil, err
			}
		}
	}
	if _, err = fmt.Fprintf(conn, "\r\n"); err != nil {
		return nil, err
	}
	if req.Body != nil {
		_, err := io.Copy(conn, req.Body)
		if err != nil {
			return nil, err
		}
	}
	trace := httptrace.ContextClientTrace(req.Context())
	if trace != nil && trace.WroteRequest != nil {
		// TODO: timing is correct?
		trace.WroteRequest(httptrace.WroteRequestInfo{
			// TODO: Err is always nil
			Err: nil,
		})
	}

	go func() {
		<-req.Context().Done()
		conn.Close()
	}()
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	select {
	case <-req.Context().Done():
		return nil, context.Canceled
	default:
	}
	if err != nil {
		return nil, err
	}
	resp.TLS = tlsConnectionState
	return resp, nil
}
