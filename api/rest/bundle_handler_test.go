package rest

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bundlesEndpoint = "/diagnostics"
	bundleEndpoint = bundlesEndpoint + "/{uuid}"
)

func TestIfReturns507ForNotExistingDir(t *testing.T) {
	t.Parallel()
	bh := BundleHandler{workDir: "not existing dir"}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
	require.NoError(t, err)

	handler := http.HandlerFunc(bh.List)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInsufficientStorage, rr.Code)

	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":507,"error":"could not read work dir: `))
}

func TestIfReturnsEmptyListWhenDirIsEmpty(t *testing.T) {
	t.Parallel()

	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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
	require.NoError(t, err)
	_, err = ioutil.TempFile(workdir, "")
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		err = os.Mkdir(filepath.Join(workdir, fmt.Sprintf("bundle-%d", i)), dirPerm)
		require.NoError(t, err)
	}

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint, nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint+"/bundle", nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint+"/bundle", nil)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	assert.True(t, strings.HasPrefix(rr.Body.String(),
		`{"code":404,"error":"bundle not found: could not stat data file bundle: `), rr.Body.String())

}

func TestIfGetReturns404WhenBundleStateIsNotJson(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-state-not-json")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`invalid JSON`), filePerm)
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint+"/bundle-state-not-json", nil)
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Get)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":404,"error":"bundle not found: could not unmarshal state file bundle-state-not-json:`), rr.Body.String())
}

func TestIfDeleteReturns404WhenNoBundleFound(t *testing.T) {
	t.Parallel()

	bh := BundleHandler{}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/not-existing-bundle", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":404,"error":"could not find bundle not-existing-bundle: `), rr.Body.String())

}

func TestIfDeleteReturns404WhenNoBundleStateFound(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "not-existing-bundle-state")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/not-existing-bundle-state", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":404,"error":"could not find bundle not-existing-bundle-state: `), rr.Body.String())
}

func TestIfDeleteReturns404WhenBundleStateIsNotJson(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-state-not-json")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	stateFilePath := filepath.Join(bundleWorkDir, stateFileName)
	err = ioutil.WriteFile(stateFilePath,
		[]byte(`invalid JSON`), filePerm)
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/bundle-state-not-json", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":404,"error":"could not find bundle bundle-state-not-json: `), rr.Body.String())
}

func TestIfDeleteReturns304WhenBundleWasDeletedBefore(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/deleted-bundle", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotModified, rr.Code)
	assert.JSONEq(t, bundleState, rr.Body.String())
}

func TestIfDeleteReturns500WhenBundleCouldNotBeDeleted(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/missing-data-file", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Delete)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":500,"error":"could not Delete bundle missing-data-file: `), rr.Body.String())
}

func TestIfDeleteReturns200WhenBundleWasDeleted(t *testing.T) {
	t.Parallel()
	workdir, err := ioutil.TempDir("", "work-dir")
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("DELETE", bundlesEndpoint+"/bundle-0", nil)
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
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle")
	err = os.Mkdir(bundleWorkDir, dirPerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(bundleWorkDir, dataFileName),
		[]byte(`OK`), filePerm)
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint+"/bundle", nil)
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
	require.NoError(t, err)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("GET", bundlesEndpoint+"/bundle", nil)
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

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("PUT", bundlesEndpoint+"/bundle-0", nil)
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
	require.NoError(t, err)
	bundleWorkDir := filepath.Join(workdir, "bundle-0")
	err = ioutil.WriteFile(bundleWorkDir, []byte{}, 0000)

	bh := BundleHandler{workDir: workdir}

	req, err := http.NewRequest("PUT", bundlesEndpoint+"/bundle-0", nil)
	require.NoError(t, err)

	// Need to Create a router that we can pass the request through so that the vars will be added to the context
	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInsufficientStorage, rr.Code)
	assert.True(t, strings.HasPrefix(rr.Body.String(), `{"code":507,"error":"could not Create bundle bundle-0 workdir: `), rr.Body.String())
}
