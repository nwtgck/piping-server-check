package http1_0_round_tripper

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
)

type Http10RoundTripper struct{}

type readerHook struct {
	r io.Reader
}

func (r readerHook) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	fmt.Println("!!! hooke", string(p[:n]))
	return n, err
}

func (rt Http10RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn net.Conn
	var err error
	if req.URL.Scheme == "https" {
		conn, err = tls.Dial("tcp", req.URL.Hostname()+":"+req.URL.Port(), nil)
	} else {
		conn, err = net.Dial("tcp", req.URL.Hostname()+":"+req.URL.Port())
	}
	if err != nil {
		return nil, err
	}
	fmt.Println("!!!!! HERE 1")
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
	fmt.Println("!!!!! HERE 2")

	pr, pw := io.Pipe()
	go func() {
		fmt.Println("!!!!! start copy")
		// TODO: error
		io.Copy(pw, readerHook{r: conn})
		//bufio.NewReader(conn)
	}()

	//resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	resp, err := http.ReadResponse(bufio.NewReader(pr), req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
