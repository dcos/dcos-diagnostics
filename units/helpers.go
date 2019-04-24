package units

import (
	"context"
	"io"
)

type TimeoutReadCloser struct {
	ctx context.Context
	src io.ReadCloser
}

func (r *TimeoutReadCloser) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(p)
}

func (r *TimeoutReadCloser) Close() error {
	return r.src.Close()
}
