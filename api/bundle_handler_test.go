package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIfReturns507ForNotExistingDir(t *testing.T) {
	bh := bundleHandler{workDir: "not existing dir"}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInsufficientStorage, rr.Code)

	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":507,"error":"could not read work dir: `))
}

func TestIfReturnsEmptyListWhenDirIsEmpty(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)

	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Equal(t, `[]`, rr.Body.String())
}

func TestIfReturnsEmptyListWhenDirIsEmptyContainsNoDirs(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	_, err = ioutil.TempFile(workdir, "")
	require.NoError(t, err)

	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Equal(t, `[]`, rr.Body.String())
}

func TestIfDirsAsBundlesIdsWithStatusUnknown(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		err = os.Mkdir(filepath.Join(workdir, fmt.Sprintf("bundle-%d", i)), 0600)
		require.NoError(t, err)
	}

	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.JSONEq(t, `[
	{
	    "id":"bundle-0",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
	},
	{
    	"id":"bundle-1",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
  	},
  	{
	    "id":"bundle-2",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
  	}]`, rr.Body.String())
}
