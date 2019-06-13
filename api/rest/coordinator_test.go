package rest

import (
	"archive/zip"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	numGoroutine := runtime.NumGoroutine()

	client := new(MockClient)

	c := Coordinator{
		client: client,
	}

	ctx := context.TODO()

	client.On("Create", ctx, "192.0.2.1", "id").Return(&Bundle{ID: "id", Status: Started}, nil)
	client.On("Create", ctx, "192.0.2.2", "id").Return(&Bundle{ID: "id", Status: Started}, nil)
	client.On("Create", ctx, "192.0.2.3", "id").Return(&Bundle{ID: "id", Status: Started}, nil)

	client.On("Status", ctx, "192.0.2.1", "id").Return(&Bundle{ID: "id", Status: Done}, nil)
	client.On("Status", ctx, "192.0.2.2", "id").Return(&Bundle{ID: "id", Status: Done}, nil)
	client.On("Status", ctx, "192.0.2.3", "id").Return(&Bundle{ID: "id", Status: Done}, nil)

	node1 := node{IP: net.ParseIP("192.0.2.1")}
	node2 := node{IP: net.ParseIP("192.0.2.2")}
	node3 := node{IP: net.ParseIP("192.0.2.3")}

	s := c.Create(context.TODO(), "id", []node{node1, node2, node3})

	var statuses []BundleStatus

	assert.Equal(t, numberOfWorkers, runtime.NumGoroutine()-numGoroutine)

	for i := 0; i < 6; i++ {
		statuses = append(statuses, <-s)
	}

	expected := []BundleStatus{
		{ID: "id", node: node1},
		{ID: "id", node: node1, done: true},
		{ID: "id", node: node2},
		{ID: "id", node: node2, done: true},
		{ID: "id", node: node3},
		{ID: "id", node: node3, done: true},
	}

	for _, s := range statuses {
		assert.Contains(t, expected, s)
	}
}

func TestCoordinatorCreateAndCollect(t *testing.T) {
	client := MockClient{}

	c := Coordinator{
		client: &client,
	}

	ctx := context.TODO()

	bundleID := "bundle-0"
	numNodes := 3

	node1 := node{IP: net.ParseIP("192.0.2.1"), baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), baseURL: "http://192.0.2.3"}

	client.On("Create", ctx, node1.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)
	client.On("Create", ctx, node2.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)
	client.On("Create", ctx, node3.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)

	client.On("Status", ctx, node1.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)
	client.On("Status", ctx, node2.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)
	client.On("Status", ctx, node3.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)

	testZip1, err := filepath.Abs("./testdata/192.0.2.1.zip")
	require.NoError(t, err)
	testZip2, err := filepath.Abs("./testdata/192.0.2.2.zip")
	require.NoError(t, err)
	testZip3, err := filepath.Abs("./testdata/192.0.2.3.zip")
	require.NoError(t, err)

	client.On("GetFile", ctx, node1.baseURL, bundleID).Return(testZip1, nil)
	client.On("GetFile", ctx, node2.baseURL, bundleID).Return(testZip2, nil)
	client.On("GetFile", ctx, node3.baseURL, bundleID).Return(testZip3, nil)

	statuses := c.Create(ctx, "bundle-0", []node{node1, node2, node3})

	bundlePath, err := c.Collect(ctx, bundleID, numNodes, statuses)
	require.NoError(t, err)
	defer os.RemoveAll(bundlePath)
	require.NotEmpty(t, bundlePath)

	zipReader, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer zipReader.Close()

	assert.Equal(t, numNodes, len(zipReader.File))

	expectedDirs := []string{"192.0.2.1/", "192.0.2.2/", "192.0.2.3/"}
	for _, f := range zipReader.File {
		assert.Contains(t, expectedDirs, f.Name)
	}
}
