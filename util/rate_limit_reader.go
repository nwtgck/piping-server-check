package util

import (
	"io"
	"time"
)

type RateLimitReader struct {
	inner      io.Reader
	bytePerSec int
}

func NewRateLimitReader(r io.Reader, bytePerSec int) io.Reader {
	return &RateLimitReader{inner: r, bytePerSec: bytePerSec}
}

func (r *RateLimitReader) Read(p []byte) (int, error) {
	if r.bytePerSec < len(p) {
		p = p[:r.bytePerSec]
	}
	n, err := r.inner.Read(p)
	if err != nil {
		return n, err
	}
	duration := time.Duration((n*1000)/r.bytePerSec) * time.Millisecond
	time.Sleep(duration)
	return n, err
}
