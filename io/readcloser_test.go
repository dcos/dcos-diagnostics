package io

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadCloserWithContext(t *testing.T) {
	src := ioutil.NopCloser(bytes.NewReader([]byte("OK")))
	ctx, cancel := context.WithCancel(context.TODO())
	rc := ReadCloserWithContext(ctx, src)

	p := make([]byte, 2)

	n, err := rc.Read(p)

	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, "OK", string(p))

	cancel()

	n, err = rc.Read(p)

	assert.Zero(t, n)
	assert.Equal(t, "OK", string(p))
	assert.EqualError(t, err, "context canceled")

	assert.NoError(t, rc.Close())
}
