package api

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadFileNoFile(t *testing.T) {
	r, err := readFile("/noFile")
	assert.Error(t, err)
	assert.Nil(t, r)
}

func TestReadFile(t *testing.T) {
	// create a test file
	tempFile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString("test")
	require.NoError(t, err)

	r, err := readFile(filepath.Join(tempFile.Name()))
	if err == nil {
		defer r.Close()
	}

	assert.NotNil(t, r)
	assert.NoError(t, err)
	buf := new(bytes.Buffer)
	io.Copy(buf, r)
	assert.Equal(t, buf.String(), "test")
}

func TestReadJournalOutputSince_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	r, err := readJournalOutputSince("", "")
	assert.Nil(t, r)
	assert.EqualError(t, err, "there is no journal on Windows")
}

func TestReadJournalOutputSince_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}

	r, err := readJournalOutputSince("not-existing.service", "")
	require.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Empty(t, data)
}
