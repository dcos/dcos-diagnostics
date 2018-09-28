package units

import (
	"io/ioutil"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadJournalOutputSince_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	r, err := ReadJournalOutputSince("", "")
	assert.Nil(t, r)
	assert.EqualError(t, err, "there is no journal on Windows")
}

func TestReadJournalOutputSince_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}

	r, err := ReadJournalOutputSince("not-existing.service", "")
	require.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Empty(t, data)
}
