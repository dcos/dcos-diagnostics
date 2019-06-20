package rest

import (
	"archive/zip"
	"context"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	client := new(MockClient)
	interval := time.Millisecond
	workDir := os.TempDir()

	c := NewParallelCoordinator(client, interval, workDir)

	ctx := context.TODO()

	localBundleID := uuid.New()

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
		id := localBundleID.String()
		client.On("CreateBundle", ctx, n.baseURL, id).Return(&Bundle{ID: id, Status: Started}, nil)
		client.On("Status", ctx, n.baseURL, id).Return(&Bundle{ID: id, Status: Done}, nil)

		expected = append(expected,
			BundleStatus{id: id, node: n},
			BundleStatus{id: id, node: n, done: true},
		)
	}
	s := c.CreateBundle(context.TODO(), localBundleID.String(), testNodes)

	var statuses []BundleStatus

	for i := 0; i < 6; i++ {
		statuses = append(statuses, <-s)
	}

	for _, s := range statuses {
		assert.Contains(t, expected, s)
	}
}

func TestCoordinatorCreateAndCollect(t *testing.T) {
	client := new(MockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	c := NewParallelCoordinator(client, interval, workDir)

	ctx := context.TODO()

	bundleID := "bundle-0"
	localBundleID := uuid.New()
	numNodes := 3

	node1 := node{IP: net.ParseIP("192.0.2.1"), Role: "agent", baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), Role: "master", baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), Role: "public_agent", baseURL: "http://192.0.2.3"}

	testZip1, err := filepath.Abs(filepath.Join("testdata", "192.0.2.1_agent.zip"))
	require.NoError(t, err)
	testZip2, err := filepath.Abs(filepath.Join("testdata", "192.0.2.2_master.zip"))
	require.NoError(t, err)
	testZip3, err := filepath.Abs(filepath.Join("testdata", "192.0.2.3_public_agent.zip"))
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
		id := localBundleID.String()
		client.On("CreateBundle", ctx, testData.n.baseURL, id).Return(&Bundle{ID: id, Status: Started}, nil)
		client.On("Status", ctx, testData.n.baseURL, id).Return(&Bundle{ID: id, Status: Done}, nil)
		client.On("GetFile", ctx, testData.n.baseURL, id, testData.zipPath).Return(nil)
	}

	statuses := c.CreateBundle(ctx, localBundleID.String(), []node{node1, node2, node3})

	bundlePath, err := c.CollectBundle(ctx, bundleID, numNodes, statuses)
	require.NoError(t, err)
	// ensure that the bundle is placed in the specified directory
	assert.True(t, filepath.HasPrefix(bundlePath, workDir))
	defer os.RemoveAll(bundlePath)
	require.NotEmpty(t, bundlePath)

	zipReader, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer zipReader.Close()

	expectedContents := []string{
		"192.0.2.1_agent/",
		"192.0.2.1_agent/test.txt",
		"192.0.2.2_master/",
		"192.0.2.2_master/test.txt",
		"192.0.2.3_public_agent/",
		"192.0.2.3_public_agent/test.txt",
	}

	filenames := []string{}
	for _, f := range zipReader.File {
		filenames = append(filenames, f.Name)
	}
	sort.Strings(filenames)

	assert.Equal(t, expectedContents, filenames)
}
