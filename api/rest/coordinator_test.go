package rest

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	client := new(TestifyMockClient)
	interval := time.Millisecond
	workDir := os.TempDir()

	c := NewParallelCoordinator(client, interval, workDir)

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

	expected := []BundleStatus{}

	for _, n := range testNodes {
		client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)
		client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil)

		expected = append(expected,
			BundleStatus{id: localBundleID, node: n},
			BundleStatus{id: localBundleID, node: n, done: true},
		)
	}
	s := c.CreateBundle(context.TODO(), localBundleID, testNodes)

	var statuses []BundleStatus

	for i := 0; i < 6; i++ {
		statuses = append(statuses, <-s)
	}

	for _, s := range statuses {
		assert.Contains(t, expected, s)
	}
}

func TestCoordinatorCreateAndCollect(t *testing.T) {
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	bundleID := "bundle-0"
	localBundleID := "bundle-local"

	node1 := node{IP: net.ParseIP("192.0.2.1"), Role: "agent", baseURL: "http://192.0.2.1"}
	node2 := node{IP: net.ParseIP("192.0.2.2"), Role: "master", baseURL: "http://192.0.2.2"}
	node3 := node{IP: net.ParseIP("192.0.2.3"), Role: "public_agent", baseURL: "http://192.0.2.3"}

	testNodes := []node{node1, node2, node3}

	failingNode := node{IP: net.ParseIP("192.0.2.4"), Role: "public_agent", baseURL: "http://192.0.2.4"}
	nodeInProgress := node{IP: net.ParseIP("192.0.2.5"), Role: "public_agent", baseURL: "http://192.0.2.5"}

	testNodes = append(testNodes, failingNode, nodeInProgress)

	ctx, cancel := context.WithTimeout(context.TODO(), 100 * time.Millisecond)
	downloaded := make(chan bool)

	client := &MockClient{
		createBundle: func(ctx context.Context, node string, ID string) (bundle *Bundle, e error) {
			return &Bundle{ID: localBundleID, Status: Started}, nil
		},
		status: func(ctx context.Context, node string, ID string) (bundle *Bundle, e error) {
			if node == nodeInProgress.baseURL {
				return  &Bundle{ID: localBundleID, Status: InProgress}, nil
			}
			return &Bundle{ID: localBundleID, Status: Done}, nil
		},
		getFile: func(ctx context.Context, node string, ID string, path string) (err error) {
			defer func() { downloaded <- true }()
			if node == failingNode.baseURL {
				return fmt.Errorf("some error")
			}
			return nil
		},
	}

	go func() {
		count := 0
		for range downloaded {
			count++
			if count == 4 {
				break
			}
		}
		cancel()
	}()

	c := NewParallelCoordinator(client, time.Microsecond, workDir)

	statuses := c.CreateBundle(ctx, localBundleID, testNodes)

	bundlePath, err := c.CollectBundle(ctx, bundleID, len(testNodes), statuses)
	require.NoError(t, err)
	// ensure that the bundle is placed in the specified directory
	assert.True(t, filepath.HasPrefix(bundlePath, workDir))
	defer os.RemoveAll(bundlePath)
	require.NotEmpty(t, bundlePath)

	zipReader, err := zip.OpenReader(bundlePath)
	require.NoError(t, err)
	defer zipReader.Close()

	expectedFiles := map[string]string{
		filepath.Join("192.0.2.1_agent", "test.txt"):        "test\n",
		filepath.Join("192.0.2.2_master", "test.txt"):       "test\n",
		filepath.Join("192.0.2.3_public_agent", "test.txt"): "test\n",
		summaryErrorsReportFileName: "error\nerror\nerror\n",
		reportFileName: `{"id":"bundle-0","nodes":{"192.0.2.1":{"status":"Done"},"192.0.2.2":{"status":"Done"},"192.0.2.3":{"status":"Done"},"192.0.2.4":{"status":"Failed","error":"some error"},"192.0.2.5":{"status":"Failed","error":"bundle creation context finished before bundle creation finished"}}}`,
	}

	files := map[string]string{}
	for _, f := range zipReader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		raw, err := ioutil.ReadAll(rc)
		assert.NoError(t, err)
		files[f.Name] = string(raw)
	}

	assert.Equal(t, expectedFiles, files)
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
	rc, err := appendToZip(zipWriter, invalidZipPath)
	assert.Nil(t, rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zip: not a valid zip file")
}

func TestHandlingForBundleUpdateInProgress(t *testing.T) {
	client := new(TestifyMockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := NewParallelCoordinator(client, interval, workDir)
	ctx := context.TODO()

	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)

	// The `Once`s here are necessary for it to find the calls in the expected order
	client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: InProgress}, nil).Once()
	client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil).Once()

	statuses := c.CreateBundle(ctx, localBundleID, []node{n})

	expected := []BundleStatus{
		{
			id:   localBundleID,
			node: n,
			done: false,
		},
		{
			id:   localBundleID,
			node: n,
			done: false,
		},
		{
			id:   localBundleID,
			node: n,
			done: true,
		},
	}

	results := []BundleStatus{}

	for i := 0; i < len(expected); i++ {
		results = append(results, <-statuses)
	}

	for _, s := range results {
		assert.Contains(t, expected, s)
	}
}

func TestErrorHandlingFromClientCreateBundle(t *testing.T) {
	client := new(TestifyMockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := NewParallelCoordinator(client, interval, workDir)
	ctx := context.TODO()
	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	expectedErr := errors.New("this stands in for any of the possible errors CreateBundle could throw")
	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(nil, expectedErr)

	s := c.CreateBundle(ctx, localBundleID, []node{n})

	expected := BundleStatus{
		id:   localBundleID,
		node: n,
		done: true,
		err:  fmt.Errorf("could not create bundle: %s", expectedErr),
	}

	actual := <-s
	assert.Equal(t, expected, actual)
}

func TestErrorHandlingFromClientStatus(t *testing.T) {
	client := new(TestifyMockClient)
	interval := time.Millisecond
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := NewParallelCoordinator(client, interval, workDir)
	ctx := context.TODO()

	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	expectedErr := errors.New("this stands in for any of the possible errors Status could throw")

	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)

	// The `Once`s here are necessary for it to find the calls in the expected order
	client.On("Status", ctx, n.baseURL, localBundleID).Return(nil, expectedErr).Once()
	client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Done}, nil).Once()

	statuses := c.CreateBundle(ctx, localBundleID, []node{n})

	expected := []BundleStatus{
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

	results := []BundleStatus{}

	for i := 0; i < len(expected); i++ {
		results = append(results, <-statuses)
	}

	for _, s := range results {
		assert.Contains(t, expected, s)
	}
}

func TestHandlingForCanceledContext(t *testing.T) {
	client := new(TestifyMockClient)
	workDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	localBundleID := "bundle-0"

	c := NewParallelCoordinator(client, time.Nanosecond, workDir)

	ctx, _ := context.WithTimeout(context.TODO(), 10*time.Millisecond)

	n := node{IP: net.ParseIP("127.0.0.1"), Role: "master", baseURL: "http://127.0.0.1"}

	client.On("CreateBundle", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: Started}, nil)

	// stay in progress forever until the context is canceled
	client.On("Status", ctx, n.baseURL, localBundleID).Return(&Bundle{ID: localBundleID, Status: InProgress}, nil)

	statuses := c.CreateBundle(ctx, localBundleID, []node{n})

	var results []BundleStatus

	for s := range statuses {
		results = append(results, s)
		if s.done {
			break
		}
	}

	expected := []BundleStatus{
		{
			id:   localBundleID,
			node: n,
			done: false,
		},
		// when the context is canceled, the response should be done with an error
		{
			id:   localBundleID,
			node: n,
			done: true,
			err:  errors.New(contextDoneErrMsg),
		},
		// when the context is canceled, the response should be done with an error
		{
			id:   localBundleID,
			node: n,
			done: false,
			err:  errors.New("could not check status: context deadline exceeded"),
		},
	}

	for _, s := range results {
		assert.Contains(t, expected, s)
	}
}
