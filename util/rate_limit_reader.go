package util

import (
	"fmt"
	"io"
	"time"
)

type RateLimitReader struct {
	inner      io.Reader
	bytePerSec int
}

func NewRateLimitReader(r io.Reader, bytePerSec int) io.Reader {
	if bytePerSec == 0 {
		panic(fmt.Errorf("bytePerSec is 0"))
	}
	return &RateLimitReader{inner: r, bytePerSec: bytePerSec}
}

func (r *RateLimitReader) Read(p []byte) (int, error) {
	startTime := time.Now()
	if r.bytePerSec < len(p) {
		p = p[:r.bytePerSec]
	}
	n, err := r.inner.Read(p)
	if err != nil {
		return n, err
	}
	duration := time.Duration((n*1000000000)/r.bytePerSec) - time.Now().Sub(startTime)
	time.Sleep(duration)
	return n, err
}
