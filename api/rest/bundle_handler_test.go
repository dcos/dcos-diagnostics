package rest

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dcos/dcos-diagnostics/collector"

	"github.com/gorilla/mux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bundleEndpoint     = bundlesEndpoint + "/{id}"
	bundleFileEndpoint = bundleEndpoint + "/file"
)

func TestIfReturnsEmptyListWhenDirIsEmpty(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Equal(t, `[]`, rr.Body.String())
}

func TestIfReturnsEmptyListWhenDirIsEmptyContainsNoDirs(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	_, err = ioutil.TempFile(workdir, "")
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Equal(t, `[]`, rr.Body.String())
}

func TestIfDirsAsBundlesIdsWithStatusUnknown(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		err = os.Mkdir(filepath.Join(workdir, fmt.Sprintf("bundle-%d", i)), dirPerm)
		require.NoError(t, err)
	}

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

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

func TestIfListShowsStatusWithoutAFile(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

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

func TestIfListWorksWithoutBundleDir(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.EqualValues(t, []byte(`[]`), rr.Body.Bytes())
}

func TestIfShowsStatusWithoutAFileButStatusDoneShouldChangeStatusToUnknown(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

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

func TestIfShowsStatusWithFileAndDontUpdatesFileSize(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	oldState := `{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`
	err = ioutil.WriteFile(stateFilePath, []byte(oldState), filePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName), []byte(`OK`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

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
	assert.JSONEq(t, oldState, string(newState))
}

func TestIfGetShowsStatusWithoutAFileWhenBundleIsDeleted(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	assert.JSONEq(t, `{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z"
	}`, rr.Body.String())
}

func TestIfGetShowsStatusWithoutAFileWhenBundleIsDone(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, stateFileName),
		[]byte(`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	assert.Contains(t,
		rr.Body.String(),
		`{"id":"bundle","status":"Unknown","started_at":"1991-05-21T00:00:00Z","stopped_at":"2019-05-21T00:00:00Z","errors":["could not stat data file bundle: `,
	)
}

func TestIfGetReturns500WhenBundleStateIsNotJson(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-state-not-json")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`invalid JSON`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-state-not-json", nil)
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.JSONEq(t,
		`{
   			"id":"bundle-state-not-json",
			"status":"Unknown",
   			"started_at":"0001-01-01T00:00:00Z",
   			"stopped_at":"0001-01-01T00:00:00Z",
   			"errors":[
      			"could not unmarshal state file bundle-state-not-json: invalid character 'i' looking for beginning of value"
   			]
		}`,
		rr.Body.String(),
	)
}

func TestIfDeleteReturns404WhenNoBundleFound(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Nanosecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/not-existing-bundle", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Equal(t, rr.Body.String(), "404 page not found\n")

}

func TestIfDeleteReturns500WhenNoBundleStateFound(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "not-existing-bundle-state")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/not-existing-bundle-state", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t,
		rr.Body.String(),
		`{"id":"not-existing-bundle-state","status":"Unknown","started_at":"0001-01-01T00:00:00Z","stopped_at":"0001-01-01T00:00:00Z","errors":["could not read state file for bundle not-existing-bundle-state:`,
	)
}

func TestIfDeleteReturns500WhenBundleStateIsNotJson(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-state-not-json")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`invalid JSON`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/bundle-state-not-json", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.JSONEq(t,
		`{
   			"id":"bundle-state-not-json",
   			"status":"Unknown",
   			"started_at":"0001-01-01T00:00:00Z",
   			"stopped_at":"0001-01-01T00:00:00Z",
   			"errors":[
      			"could not unmarshal state file bundle-state-not-json: invalid character 'i' looking for beginning of value"
   			]
		}`,
		rr.Body.String(),
	)
}

func TestIfDeleteReturns304WhenBundleWasDeletedBefore(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "deleted-bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	bundleState := `{
		"id": "bundle",
		"status": "Deleted",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`
	err = ioutil.WriteFile(stateFilePath, []byte(bundleState), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/deleted-bundle", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotModified, rr.Code)
	assert.JSONEq(t, bundleState, rr.Body.String())
}

func TestIfDeleteReturns500WhenBundleFileIsMissing(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "missing-data-file")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath, []byte((`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`)), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/missing-data-file", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t,
		rr.Body.String(),
		`{"id":"bundle","status":"Unknown","started_at":"1991-05-21T00:00:00Z","stopped_at":"2019-05-21T00:00:00Z","errors":["could not stat data file missing-data-file: `,
		rr.Body.String(),
	)
}

func TestIfDeleteReturns200WhenBundleWasDeleted(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-0")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath, []byte((`{
		"id": "bundle-0",
		"status": "Done",
		"size": 2,
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`)), filePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName), []byte(`OK`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/bundle-0", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"id": "bundle-0",
		"status": "Deleted",
		"size": 2,
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`, rr.Body.String())
}

func TestIfGetFileReturnsBundle(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName),
		[]byte(`OK`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.GetFile)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())

}

func TestIfGetFileReturnsErrorWhenBundleDoesNotExists(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.GetFile)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Equal(t, "404 page not found\n", rr.Body.String())

}

func TestIfCreateReturns409WhenBundleWithGivenIdAlreadyExists(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-0")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath, []byte((`{
		"id": "bundle-0",
		"status": "Done",
		"size": 2,
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`)), filePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName), []byte(`OK`), filePerm)
	require.NoError(t, err)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.JSONEq(t, `{"code":409,"error":"bundle bundle-0 already exists"}`, rr.Body.String())

}

func TestIfCreateReturns507WhenCouldNotCreateWorkDir(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-0")
	err = ioutil.WriteFile(bundleWorkDir, []byte{}, 0000)

	bh, err := NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInsufficientStorage, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":507,"error":"could not create bundle bundle-0 workdir: `)
}

func TestIfE2E_(t *testing.T) {
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir) // just check if dcos-diagnostics will create whole path to workdir
	require.NoError(t, err)

	now, err := time.Parse(time.RFC3339, "2015-08-05T08:40:51.620Z")
	require.NoError(t, err)

	bh, err := NewBundleHandler(
		workdir,
		[]collector.Collector{
			MockCollector{name: "collector-1", err: fmt.Errorf("some error")},
			MockCollector{name: "collector-2", rc: ioutil.NopCloser(bytes.NewReader([]byte("OK")))},
		},
		time.Second,
	)
	require.NoError(t, err)
	bh.clock = &MockClock{now: now}

	router := mux.NewRouter()
	router.HandleFunc(bundlesEndpoint, bh.List).Methods(http.MethodGet)
	router.HandleFunc(bundleEndpoint, bh.Create).Methods(http.MethodPut)
	router.HandleFunc(bundleEndpoint, bh.Get).Methods(http.MethodGet)
	router.HandleFunc(bundleEndpoint, bh.Delete).Methods(http.MethodDelete)
	router.HandleFunc(bundleFileEndpoint, bh.GetFile).Methods(http.MethodGet)

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	client := NewDiagnosticsClient(testServer.Client())

	t.Run("get status of not existing bundle-0", func(t *testing.T) {
		bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
		assert.Nil(t, bundle)
		assert.IsType(t, &DiagnosticsBundleNotFoundError{}, err)
	})

	t.Run("create bundle-0", func(t *testing.T) {
		bundle, err := client.CreateBundle(context.TODO(), testServer.URL, "bundle-0")
		require.NoError(t, err)

		assert.Equal(t, &Bundle{
			ID:      "bundle-0",
			Status:  Started,
			Started: now.Add(time.Hour),
		}, bundle)
	})

	t.Run("get bundle-0 status", func(t *testing.T) {
		for { // busy wait for bundle
			bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
			//require.NoError(t, err)
			assert.NoError(t, err)
			if bundle.Status == Done {
				break
			}
		}

		bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
		require.NoError(t, err)

		assert.Equal(t, &Bundle{
			ID:      "bundle-0",
			Status:  Done,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		}, bundle)
	})

	t.Run("get bundle-0 file and validate it", func(t *testing.T) {

		f, err := ioutil.TempFile("", "*.zip")
		require.NoError(t, err)
		f.Close()
		defer os.Remove(f.Name())

		err = client.GetFile(context.TODO(), testServer.URL, "bundle-0", f.Name())
		require.NoError(t, err)

		reader, err := zip.OpenReader(f.Name())
		require.NoError(t, err)

		require.Len(t, reader.File, 3)
		assert.Equal(t, "collector-2", reader.File[0].Name)
		assert.Equal(t, "summaryReport.txt", reader.File[1].Name)
		assert.Equal(t, "summaryErrorsReport.txt", reader.File[2].Name)

		rc, err := reader.File[0].Open()
		require.NoError(t, err)
		content, err := ioutil.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "OK", string(content))

		rc, err = reader.File[1].Open()
		require.NoError(t, err)
		content, err = ioutil.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t,
			`[START GET collector-1]
[STOP GET collector-1]
[START GET collector-2]
[STOP GET collector-2]
`, string(content))

		rc, err = reader.File[2].Open()
		require.NoError(t, err)
		content, err = ioutil.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "could not collect collector-1: some error", string(content))
	})

	t.Run("delete bundle-0", func(t *testing.T) {

		req, err := http.NewRequest(http.MethodDelete, testServer.URL+bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)

		rr, err := http.DefaultClient.Do(req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.StatusCode)

		body, err := ioutil.ReadAll(rr.Body)
		require.NoError(t, err)

		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Deleted,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		})), string(body))
	})

	t.Run("list bundles", func(t *testing.T) {
		time.Sleep(10 * time.Millisecond)
		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal([]Bundle{{
			ID:      "bundle-0",
			Status:  Deleted,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		}})), rr.Body.String())
	})
}

func TestBundleDirIsCreatedIfNotExists(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir)
	require.NoError(t, err)

	_, err = NewBundleHandler(workdir, nil, time.Millisecond)
	require.NoError(t, err)

	assert.DirExists(t, workdir)
}

// MockClock is a monotonic clock. Every call to Now() adds one hour
type MockClock struct {
	now time.Time
}

func (m *MockClock) Now() time.Time {
	m.now = m.now.Add(time.Hour)
	return m.now
}

type MockCollector struct {
	name     string
	optional bool
	rc       io.ReadCloser
	err      error
}

func (m MockCollector) Name() string {
	return m.name
}

func (m MockCollector) Optional() bool {
	return m.optional
}

func (m MockCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	return m.rc, m.err
}
