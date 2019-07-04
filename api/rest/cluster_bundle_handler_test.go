package rest

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteBundleCreationConflictErrorWhenBundleExists(t *testing.T) {
}

func TestRemoteBundleCreationFileSystemError(t *testing.T) {
}

func TestDeleteBundleOnOtherMaster(t *testing.T) {
}

func TestDeleteAlreadyDeletedBundle(t *testing.T) {
}

func TestDeleteBundleThatNeverExisted(t *testing.T) {
}

func TestDeleteUnreadableBundle(t *testing.T) {
}

func TestStatusForBundleOnOtherMaster(t *testing.T) {
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir) // just check if dcos-diagnostics will create whole path to workdir
	require.NoError(t, err)

	now, err := time.Parse(time.RFC3339, "2015-08-05T08:40:51.620Z")
	require.NoError(t, err)

	tools := new(MockedTools)
	tools.On("GetMasterNodes").Return([]dcos.Node{
		{
			Role: "master",
			IP:   "192.0.2.2",
		},
		{
			Role: "master",
			IP:   "192.0.2.4",
		},
		{
			Role: "master",
			IP:   "192.0.2.5",
		},
	}, nil)

	ctx := context.TODO()

	client := new(MockClient)
	client.On("Status", ctx, "http://192.0.2.2", "bundle-0").Return(nil, fmt.Errorf("asdf"))
	client.On("Status", ctx, "http://192.0.2.4", "bundle-0").Return(&Bundle{
		ID:      "bundle-0",
		Status:  Done,
		Started: now,
		Stopped: now.Add(1 * time.Hour),
	}, nil)
	client.On("Status", ctx, "http://192.0.2.5", "bundle-0").Return(nil, fmt.Errorf("asdf"))

	coord := new(mockCoordinator)
	bh := NewClusterBundleHandler(
		coord,
		client,
		tools,
		workdir,
		time.Second,
		&MockClock{now: now},
		MockURLBuilder{},
	)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Status).Methods(http.MethodGet)

	req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, string(jsonMarshal(Bundle{
		ID:      "bundle-0",
		Status:  Done,
		Started: now.Add(time.Hour),
	})), rr.Body.String())
}

func TestStatusOnMissingBundle(t *testing.T) {
}

func TestStatusForUnreadableBunde(t *testing.T) {
}

func TestDownloadBundleOnOtherMaster(t *testing.T) {
}

func TestDownloadMissingBundle(t *testing.T) {
}

func TestDownloadUnreadableBundle(t *testing.T) {
}

func TestListWithBundlesOnOtherMasters(t *testing.T) {
}

func TestRemoteBundleCreation(t *testing.T) {
	workdir, err := ioutil.TempDir("", "work-dir")
	require.NoError(t, err)
	err = os.RemoveAll(workdir) // just check if dcos-diagnostics will create whole path to workdir
	require.NoError(t, err)

	now, err := time.Parse(time.RFC3339, "2015-08-05T08:40:51.620Z")
	require.NoError(t, err)

	tools := new(MockedTools)
	tools.On("GetMasterNodes").Return([]dcos.Node{
		{
			Leader: true,
			Role:   "master",
			IP:     "192.0.2.2",
		},
	}, nil)
	tools.On("GetAgentNodes").Return([]dcos.Node{
		{
			Role: "agent",
			IP:   "192.0.2.1",
		},
		{
			Role: "agent",
			IP:   "192.0.2.3",
		},
	}, nil)

	ctx := context.TODO()

	client := new(MockClient)
	client.On("Status", ctx, "http://192.0.2.2", "bundle-0").Return(nil, fmt.Errorf("asdf"))

	coord := new(mockCoordinator)
	bh := NewClusterBundleHandler(
		coord,
		client,
		tools,
		workdir,
		time.Second,
		&MockClock{now: now},
		MockURLBuilder{},
	)

	router := mux.NewRouter()
	router.HandleFunc(bundleEndpoint, bh.Create).Methods(http.MethodPut)
	router.HandleFunc(bundleEndpoint, bh.Status).Methods(http.MethodGet)
	router.HandleFunc(bundleEndpoint, bh.Delete).Methods(http.MethodDelete)
	router.HandleFunc(bundleFileEndpoint, bh.Download).Methods(http.MethodGet)

	t.Run("send creation request", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Status:  Started,
			Started: now.Add(time.Hour),
		})), rr.Body.String())

	})

	t.Run("get status", func(t *testing.T) {
		rr := httptest.NewRecorder()

		tries := 0
		retryLimit := 100
		for { // busy wait for bundle
			time.Sleep(time.Millisecond)
			req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
			require.NoError(t, err)

			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if strings.Contains(rr.Body.String(), Done.String()) {
				break
			}
			tries++
			// keeps the test suite from hanging if something is wrong
			require.True(t, tries < retryLimit, "status wait loop exceeded retry limit")
		}

		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0", nil)
		require.NoError(t, err)

		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, string(jsonMarshal(Bundle{
			ID:      "bundle-0",
			Size:    727,
			Status:  Done,
			Started: now.Add(time.Hour),
			Stopped: now.Add(2 * time.Hour),
			Errors:  []string{},
		})), rr.Body.String())
	})

	t.Run("get bundle-0 file and validate it", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, bundlesEndpoint+"/bundle-0/file", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		reader, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(len(rr.Body.Bytes())))
		require.NoError(t, err)

		expectedContents := []string{
			"192.0.2.1/",
			"192.0.2.1/test.txt",
			"192.0.2.2/",
			"192.0.2.2/test.txt",
			"192.0.2.3/",
			"192.0.2.3/test.txt",
		}

		filenames := []string{}
		for _, f := range reader.File {
			filenames = append(filenames, f.Name)
		}
		sort.Strings(filenames)

		assert.Equal(t, expectedContents, filenames)
	})

	t.Run("delete bundle", func(t *testing.T) {
		//TODO: implement this
		assert.True(t, false)
	})
}

type mockCoordinator struct{}

func (c mockCoordinator) CreateBundle(ctx context.Context, id string, nodes []node) <-chan BundleStatus {
	statuses := make(chan BundleStatus, len(nodes))

	for _, n := range nodes {
		node := n
		statuses <- BundleStatus{
			id:   id,
			node: node,
			done: true,
		}
	}

	return statuses
}

func (c mockCoordinator) CollectBundle(ctx context.Context, id string, numBundles int, statuses <-chan BundleStatus) (string, error) {
	return filepath.Abs(filepath.Join("testdata", "combined.zip"))
}

type MockURLBuilder struct{}

func (m MockURLBuilder) BaseURL(ip net.IP, _ string) (string, error) {
	return fmt.Sprintf("http://%s", ip), nil
}
