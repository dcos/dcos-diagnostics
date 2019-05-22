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
		err = os.Mkdir(filepath.Join(workdir, fmt.Sprintf("bundle-%d", i)), 0700)
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
		"status": "Unknown",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
	},
	{
    	"id":"bundle-1",
		"status": "Unknown",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
  	},
  	{
	    "id":"bundle-2",
		"status": "Unknown",
	    "started_at":"0001-01-01T00:00:00Z",
	    "stopped_at":"0001-01-01T00:00:00Z"
  	}]`, rr.Body.String())
}

func TestIfShowsStatusWithoutAFile(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, 0700)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), 0600)
	require.NoError(t, err)


	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.JSONEq(t, `[{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z"
	}]`, rr.Body.String())
}

func TestIfShowsStatusWithoutAFileButStatusDoneShouldChangeStatusToUnknown(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, 0700)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), 0600)
	require.NoError(t, err)


	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.JSONEq(t, `[{
		"id": "bundle",
		"status": "Unknown",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z"
	}]`, rr.Body.String())
}

func TestIfShowsStatusWithFileAndUpdatesFileSize(t *testing.T) {

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, 0700)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), 0600)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName), []byte(`OK`), 0600)
	require.NoError(t, err)


	bh := bundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", baseRoute+reportDiagnostics, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.list)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	expectedState := `{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z",
		"size": 2
	}`

	assert.JSONEq(t, "["+expectedState+"]", rr.Body.String())

	newState, err := ioutil.ReadFile(stateFilePath)
	assert.JSONEq(t, expectedState, string(newState))
}