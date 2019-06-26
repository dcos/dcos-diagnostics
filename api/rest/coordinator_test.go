package rest

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	client := new(MockClient)
	interval := time.Millisecond
	workDir := os.TempDir()

	c := newParallelCoordinator(client, interval, workDir)

	ctx := context.TODO()

	localBundleID := "bundle-0"

	node1 := node{IP: net.ParseIP("192.0.2.1"), baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), baseURL: "http://192.0.2.3"}
	testNodes := []node{
		node1,
		node2,
		node3,
	}

	expected := []bundleStatus{}

	for _, n := range testNodes {
		client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)
		client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil)

		expected = append(expected,
			bundleStatus{id: localBundleID, node: n},
			bundleStatus{id: localBundleID, node: n, done: true},
		)
	}
	s := c.CreateBundle(context.TODO(), localBundleID, testNodes)

	var statuses []bundleStatus

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

	c := newParallelCoordinator(client, interval, workDir)

	ctx := context.TODO()

	bundleID := "bundle-0"
	localBundleID := "bundle-local"
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
		client.On("CreateBundle", ctx, testData.n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)
		client.On("Status", ctx, testData.n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil)
		client.On("GetFile", ctx, testData.n.baseURL, localBundleID, testData.zipPath).Return(nil)
	}

	statuses := c.CreateBundle(ctx, localBundleID, []node{node1, node2, node3})

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

func TestAppendToZipErrorsWithMalformedZip(t *testing.T) {

	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	bundlePath := filepath.Join(testDataDir, "test-bundle.zip")

	testZip, err := os.Create(bundlePath)
	require.NoError(t, err)
	defer os.Remove(bundlePath)
	defer testZip.Close()

	zipWriter := zip.NewWriter(testZip)
	defer zipWriter.Close()

	invalidZipPath := filepath.Join(testDataDir, "not_a_zip.txt")
	err = appendToZip(zipWriter, invalidZipPath)
	require.Error(t, err)
}

func TestErrorHandlingFromClientCreateBundle(t *testing.T) {
	client := new(MockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := newParallelCoordinator(client, interval, workDir)
	ctx := context.TODO()
	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	expectedErr := errors.New("this stands in for any of the possible errors CreateBundle could throw")
	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(nil, expectedErr)

	s := c.CreateBundle(ctx, localBundleID, []node{n})

	expected := bundleStatus{
		id:   localBundleID,
		node: n,
		done: true,
		err:  fmt.Errorf("could not create bundle: %s", expectedErr),
	}

	actual := <-s
	assert.Equal(t, expected, actual)
}

func TestErrorHandlingFromClientStatus(t *testing.T) {
	client := new(MockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := newParallelCoordinator(client, interval, workDir)
	ctx := context.TODO()

	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	expectedErr := errors.New("this stands in for any of the possible errors Status could throw")

	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)

	// The `Once`s here are necessary for it to find the calls in the expected order
	client.On("Status", ctx, n.baseURL, localBundleID).Return(nil, expectedErr).Once()
	client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil).Once()

	statuses := c.CreateBundle(ctx, localBundleID, []node{n})

	expected := []bundleStatus{
		{
			id:   localBundleID,
			node: n,
			done: false,
		},
		{
			id:   localBundleID,
			node: n,
			done: false,
			err:  fmt.Errorf("could not check status: %s", expectedErr),
		},
		{
			id:   localBundleID,
			node: n,
			done: true,
		},
	}

	for i := 0; i < len(expected); i++ {
		e := expected[i]
		status := <-statuses
		assert.Equal(t, e, status)
	}
}
