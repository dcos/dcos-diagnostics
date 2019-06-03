package collector

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/coreos/go-systemd/journal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemd_Collect(t *testing.T) {
	if os.Getenv("TRAVIS") != "" {
		t.Skipf("SKIPPING: We can not read from journal in Travis")
	}
	if !journal.Enabled() {
		t.Skipf("SKIPPING: Journal not enabled")
	}

	err := journal.Send("test message", journal.PriInfo, map[string]string{"UNIT": "test-unit"})
	require.NoError(t, err)

	c := NewSystemd(
		"test",
		false,
		"test-unit",
		time.Second,
	)
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Contains(t, string(raw), "test message")
}
