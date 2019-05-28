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
	"strings"
	"testing"
	"time"

	"github.com/dcos/dcos-diagnostics/collectors"

	"github.com/gorilla/mux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bundlesEndpoint    = "/diagnostics"
	bundleEndpoint     = bundlesEndpoint + "/{id}"
	bundleFileEndpoint = bundleEndpoint + "/file"
)

func TestIfReturns507ForNotExistingDir(t *testing.T) {
	t.Parallel()
	bh := NewBundleHandler("not existing dir", nil, time.Nanosecond)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInsufficientStorage, rr.Code)

	assert.Contains(t, rr.Body.String(), `{"code":507,"error":"could not read work dir: `)
}

func TestIfReturnsEmptyListWhenDirIsEmpty(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

func TestIfShowsStatusWithFileAndUpdatesFileSize(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`{
		"id": "bundle",
		"status": "Done",
		"started_at":"1991-05-21T00:00:00Z",
		"stopped_at":"2019-05-21T00:00:00Z" }`), filePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName), []byte(`OK`), filePerm)
	require.NoError(t, err)

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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
	assert.JSONEq(t, expectedState, string(newState))
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	assert.Contains(t, rr.Body.String(),
		`{"code":404,"error":"bundle not found: could not stat data file bundle: `)
}

func TestIfGetReturns404WhenBundleStateIsNotJson(t *testing.T) {
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-state-not-json", nil)
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":404,"error":"bundle not found: could not unmarshal state file bundle-state-not-json:`)
}

func TestIfDeleteReturns404WhenNoBundleFound(t *testing.T) {
	t.Parallel()

	bh := NewBundleHandler("", nil, time.Nanosecond)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/not-existing-bundle", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":404,"error":"bundle not found: could not read state file for bundle not-existing-bundle: `)

}

func TestIfDeleteReturns404WhenNoBundleStateFound(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	defer os.RemoveAll(workdir)
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "not-existing-bundle-state")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/not-existing-bundle-state", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":404,"error":"bundle not found: could not read state file for bundle not-existing-bundle-state: `)
}

func TestIfDeleteReturns404WhenBundleStateIsNotJson(t *testing.T) {
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/bundle-state-not-json", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":404,"error":"bundle not found: could not unmarshal state file bundle-state-not-json: `)
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

func TestIfDeleteReturns404WhenBundleFileIsMissing(t *testing.T) {
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

	req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/missing-data-file", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), `{"code":404,"error":"bundle not found: could not stat data file missing-data-file: `, rr.Body.String())
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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(workdir, nil, time.Millisecond)

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

	bh := NewBundleHandler(
		workdir,
		[]collectors.Collector{
			MockCollector{name: "collector-1", err: fmt.Errorf("some error")},
			MockCollector{name: "collector-2", rc: ioutil.NopCloser(bytes.NewReader([]byte("OK")))},
		},
		time.Second,
	)
	bh.clock = &MockClock{now: now}

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create).Methods(http.MethodPut)
	router.HandleFunc(bundleEndpoint, bh.Get).Methods(http.MethodGet)
	router.HandleFunc(bundleEndpoint, bh.Delete).Methods(http.MethodDelete)
	router.HandleFunc(bundleFileEndpoint, bh.GetFile).Methods(http.MethodGet)
	rr := httptest.NewRecorder()

	t.Run("create bundle-0", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Started,
			Started: now.Add(time.Hour),
		})), rr.Body.String())
	})

	t.Run("get bundle-0 status", func(t *testing.T) {
		for { // busy wait for bundle
			time.Sleep(time.Millisecond)
			req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
			require.NoError(t, err)
			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if strings.Contains(rr.Body.String(), Done.String()) {
				break
			}
		}

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Done,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		})), rr.Body.String())
	})

	t.Run("get bundle-0 file and validate it", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0/file", nil)
		require.NoError(t, err)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		reader, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(len(rr.Body.Bytes())))
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
		req, err := http.NewRequest(http.MethodDelete, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Deleted,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		})), rr.Body.String())
	})

	t.Run("get deleted status of bundle-0", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Deleted,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Size:    494,
			Errors:  []string{"could not collect collector-1: some error"},
		})), rr.Body.String())
	})
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
