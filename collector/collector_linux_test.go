package collector

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemd_Collect(t *testing.T) {
	if os.Getenv("TRAVIS") != "" {
		t.Skipf("SKIPPING: We can not read from journal in Travis")
	}

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

	assert.Empty(t, string(raw))
}
