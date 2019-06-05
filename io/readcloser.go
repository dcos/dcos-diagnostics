package io

import (
	"context"
	"io"
	"time"
)

// ReadCloser wraps an io.ReadCloser with one that checks ctx.Done() on each Read call.
//
// If ctx has a deadline and if r has a `SetReadDeadline(time.Time) error` method,
// then it is called with the deadline.
//
// Source : https://gist.github.com/dchapes/6c992bf3e943934462509338cd213e99
// See: https://github.com/golang/go/issues/20280
func ReadCloserWithContext(ctx context.Context, r io.ReadCloser) io.ReadCloser {
	if deadline, ok := ctx.Deadline(); ok {
		type deadliner interface {
			SetReadDeadline(time.Time) error
		}
		if d, ok := r.(deadliner); ok {
			_ = d.SetReadDeadline(deadline)
		}
	}
	return readerWithContext{ctx, r}
}

type readerWithContext struct {
	ctx context.Context
	r   io.ReadCloser
}

func (r readerWithContext) Read(p []byte) (n int, err error) {
	if err = r.ctx.Err(); err != nil {
		return
	}
	if n, err = r.r.Read(p); err != nil {
		return
	}
	err = r.ctx.Err()
	return
}

func (r readerWithContext) Close() error {
	return r.r.Close()
}
