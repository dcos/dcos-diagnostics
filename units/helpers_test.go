package units

import (
	"context"
	"io/ioutil"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dcos/dcos-diagnostics/io"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadJournalOutputSince_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	r, err := ReadJournalOutputSince(context.TODO(), "", time.Minute)
	assert.Nil(t, r)
	assert.EqualError(t, err, "there is no journal on Windows")
}

func TestReadJournalOutputSince_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}

	ctxDeadline := time.Now().Add(1 * time.Hour) // this test shouldn't take longer than an hour, should it?
	ctx, cancel := context.WithDeadline(context.Background(), ctxDeadline)
	defer cancel()
	r, err := ReadJournalOutputSince(ctx, "not-existing.service", time.Minute)
	require.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Empty(t, data)
}

func TestTimedReaderShouldTimeOut(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	src := ioutil.NopCloser(strings.NewReader("test"))
	tr := io.ReadCloserWithContext(ctx, src)
	buf := make([]byte, 1024)
	n, err := tr.Read(buf)
	assert.Equal(t, 0, n, "Expected 0 bytes from readerWithContext because it should have timed out before 1st read")
	require.Error(t, err, "Reader should have returned an error because the deadline should have been reached")
}
