package rest

import (
	"archive/zip"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	numGoroutine := runtime.NumGoroutine()

	client := new(MockClient)

	c := ParallelCoordinator{
		client: client,
	}

	ctx := context.TODO()

	node1 := node{IP: net.ParseIP("192.0.2.1"), baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), baseURL: "http://192.0.2.3"}
	testNodes := []node{
		node1,
		node2,
		node3,
	}

	expected := []BundleStatus{}

	for _, n := range testNodes {
		id := fmt.Sprintf("%s-%s", n.IP, "id")
		client.On("CreateBundle", ctx, n.baseURL, id).Return(&Bundle{ID: id, Status: Started}, nil)
		client.On("Status", ctx, n.baseURL, id).Return(&Bundle{ID: id, Status: Done}, nil)

		expected = append(expected,
			BundleStatus{ID: id, node: n},
			BundleStatus{ID: id, node: n, done: true},
		)
	}
	s := c.Create(context.TODO(), "id", testNodes)

	var statuses []BundleStatus

	assert.Equal(t, numberOfWorkers, runtime.NumGoroutine()-numGoroutine)

	for i := 0; i < 6; i++ {
		statuses = append(statuses, <-s)
	}

	for _, s := range statuses {
		assert.Contains(t, expected, s)
	}
}

func TestCoordinatorCreateAndCollect(t *testing.T) {
	//TODO(janisz): FIXME
	t.Skipf("Uncoment this test after we figure out how to generate temp local bundle dir")
	client := MockClient{}

	c := ParallelCoordinator{
		client: &client,
	}

	ctx := context.TODO()

	bundleID := "bundle-0"
	numNodes := 3

	node1 := node{IP: net.ParseIP("192.0.2.1"), baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), baseURL: "http://192.0.2.3"}

	testZip1, err := filepath.Abs(filepath.Join("testdata", "192.0.2.1.zip"))
	require.NoError(t, err)
	testZip2, err := filepath.Abs(filepath.Join("testdata", "192.0.2.2.zip"))
	require.NoError(t, err)
	testZip3, err := filepath.Abs(filepath.Join("testdata", "192.0.2.3.zip"))
	require.NoError(t, err)

	testNodes := []struct {
		n       node
		zipPath string
	}{
		{
			n:       node1,
			zipPath: testZip1,
		},
		{
			n:       node2,
			zipPath: testZip2,
		},
		{
			n:       node3,
			zipPath: testZip3,
		},
	}

	for _, testData := range testNodes {
		id := fmt.Sprintf("%s-%s", testData.n.IP, bundleID)
		//id := bundleID
		client.On("CreateBundle", ctx, testData.n.baseURL, id).Return(&Bundle{ID: id, Status: Started}, nil)
		client.On("Status", ctx, testData.n.baseURL, id).Return(&Bundle{ID: id, Status: Done}, nil)
		client.On("GetFile", ctx, testData.n.baseURL, id, testData.zipPath).Return(nil)
	}

	statuses := c.Create(ctx, "bundle-0", []node{node1, node2, node3})

	bundlePath, err := c.Collect(ctx, bundleID, numNodes, statuses)
	require.NoError(t, err)
	defer os.RemoveAll(bundlePath)
	require.NotEmpty(t, bundlePath)

	zipReader, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer zipReader.Close()

	expectedContents := []string{
		"192.0.2.1/",
		"192.0.2.1/test.txt",
		"192.0.2.2/",
		"192.0.2.2/test.txt",
		"192.0.2.3/",
		"192.0.2.3/test.txt",
	}

	filenames := []string{}
	for _, f := range zipReader.File {
		filenames = append(filenames, f.Name)
	}
	sort.Strings(filenames)

	assert.Equal(t, expectedContents, filenames)
}
