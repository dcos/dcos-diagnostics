package rest

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate(t *testing.T) {
	expectedBundle := Bundle{
		ID:      "bundle-0",
		Started: time.Now().UTC(),
		Status:  Started,
	}

	type payload struct {
		BundleType Type `json:"type"`
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)

		var args payload
		err := json.NewDecoder(r.Body).Decode(&args)
		require.NoError(t, err)

		assert.Equal(t, Local, args.BundleType)

		response := jsonMarshal(expectedBundle)
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	bundle, err := client.Create(context.TODO(), testServer.URL, expectedBundle.ID)
	require.NoError(t, err)
	assert.EqualValues(t, expectedBundle, *bundle)
}

func TestGetStatus(t *testing.T) {
	expectedResp := Bundle{
		ID:      "bundle-0",
		Started: time.Now().UTC(),
		Status:  InProgress,
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		response := jsonMarshal(expectedResp)
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
	require.NoError(t, err)
	assert.EqualValues(t, expectedResp, *bundle)
}

func TestGetFile(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0/file", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()
	client := DiagnosticsClient{
		client: testClient,
	}

	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	defer os.RemoveAll(f.Name())

	err = client.GetFile(context.TODO(), testServer.URL, "bundle-0", f.Name())
	require.NoError(t, err)

	contents, err := ioutil.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, []byte("test"), contents)
}

func TestGetStatusBundleHasStatusUnknownBundleIDNotFound(t *testing.T) {
	expected := Bundle{
		Status: Unknown,
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
	assert.NoError(t, err)
	assert.EqualValues(t, expected, *bundle)
}

func TestGetStatusReturnsErrorWhenResponseIs500(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 ERROR"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
	assert.Contains(t, err.Error(), "received unexpected status code from")
	assert.Contains(t, err.Error(), ": 500 500 ERROR")
	assert.Nil(t, bundle)
}

func TestGetStatusReturnsErrorWhenResponseIsMalformed(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not a json"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	bundle, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
	assert.EqualError(t, err, "invalid character 'o' in literal null (expecting 'u')")
	assert.Nil(t, bundle)
}


func TestGetFileReturnsErrorWhenBundleIDNotFound(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/diagnostics/bundle-0/file", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusNotFound)
	}))

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	defer os.RemoveAll(f.Name())

	err = client.GetFile(context.TODO(), testServer.URL, "bundle-0", f.Name())
	assert.Error(t, err)
}
