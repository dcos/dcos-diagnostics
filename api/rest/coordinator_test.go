package rest

import (
	"archive/zip"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
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

	node1 := node{IP: net.ParseIP("192.0.2.1"), baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), baseURL: "http://192.0.2.3"}

	client.On("Create", ctx, node1.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)
	client.On("Create", ctx, node2.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)
	client.On("Create", ctx, node3.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Started}, nil)

	client.On("Status", ctx, node1.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)
	client.On("Status", ctx, node2.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)
	client.On("Status", ctx, node3.baseURL, bundleID).Return(&Bundle{ID: bundleID, Status: Done}, nil)

	fileContents := []byte("test")

	file1, err := ioutil.TempFile("", fmt.Sprintf("%s-*.zip", bundleID))
	require.NoError(t, err)
	defer os.Remove(file1.Name())
	file1.Write(fileContents)

	file2, err := ioutil.TempFile("", fmt.Sprintf("%s-*.zip", bundleID))
	require.NoError(t, err)
	defer os.Remove(file2.Name())
	file2.Write(fileContents)

	file3, err := ioutil.TempFile("", fmt.Sprintf("%s-*.zip", bundleID))
	require.NoError(t, err)
	defer os.Remove(file3.Name())
	file3.Write(fileContents)

	client.On("GetFile", ctx, node1.baseURL, bundleID).Return(file1.Name(), nil)
	client.On("GetFile", ctx, node2.baseURL, bundleID).Return(file2.Name(), nil)
	client.On("GetFile", ctx, node3.baseURL, bundleID).Return(file3.Name(), nil)

	statuses := c.Create(ctx, "bundle-0", []node{node1, node2, node3})

	bundlePath, err := c.Collect(ctx, statuses)
	require.NoError(t, err)
	defer os.RemoveAll(bundlePath)

	zipReader, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer zipReader.Close()

}
