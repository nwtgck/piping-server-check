package util

import (
	"io"
)

type FinishableReader struct {
	inner    io.Reader
	finishCh <-chan struct{}
}

func NewFinishableReader(r io.Reader, finishCh <-chan struct{}) io.Reader {
	return &FinishableReader{inner: r, finishCh: finishCh}
}

func (r *FinishableReader) Read(p []byte) (int, error) {
	select {
	case <-r.finishCh:
		return 0, io.EOF
	default:
	}
	return r.inner.Read(p)
}
