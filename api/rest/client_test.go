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
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
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

	bundle, err := client.CreateBundle(context.TODO(), testServer.URL, expectedBundle.ID)
	require.NoError(t, err)
	assert.EqualValues(t, expectedBundle, *bundle)
}

func TestCreateShouldErrorWhenMalformedResponse(t *testing.T) {
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
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)

		var args payload
		err := json.NewDecoder(r.Body).Decode(&args)
		require.NoError(t, err)

		assert.Equal(t, Local, args.BundleType)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("malformed response"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	bundle, err := client.CreateBundle(context.TODO(), testServer.URL, expectedBundle.ID)
	assert.EqualError(t, err, "invalid character 'm' looking for beginning of value")
	assert.Nil(t, bundle)
}

func TestGetStatus(t *testing.T) {
	expectedResp := Bundle{
		ID:      "bundle-0",
		Started: time.Now().UTC(),
		Status:  InProgress,
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
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
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0/file", r.URL.Path)
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
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	_, err := client.Status(context.TODO(), testServer.URL, "bundle-0")
	assert.Error(t, err)
	assert.IsType(t, &DiagnosticsBundleNotFoundError{}, err)
}

func TestCreateReturnsErrorWhenResponseIs500(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 ERROR"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	bundle, err := client.CreateBundle(context.TODO(), testServer.URL, "bundle-0")
	assert.Contains(t, err.Error(), "received unexpected status code [500] from")
	assert.Contains(t, err.Error(), ": 500 ERROR")
	assert.Nil(t, bundle)
}

func TestGetStatusReturnsErrorWhenResponseIs500(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
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
	assert.IsType(t, &DiagnosticsBundleUnreadableError{}, err)
	assert.Nil(t, bundle)
}

func TestGetFileReturnsErrorWhenResponseIs500(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0/file", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 ERROR"))
	}))
	defer testServer.CloseClientConnections()

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}
	err := client.GetFile(context.TODO(), testServer.URL, "bundle-0", "")
	assert.IsType(t, &DiagnosticsBundleUnreadableError{}, err)
}

func TestClientReturnsErrorWhenNodeIsInvalid(t *testing.T) {
	client := DiagnosticsClient{client: http.DefaultClient}
	bundle, err := client.CreateBundle(context.TODO(), ``, "bundle-0")
	assert.EqualError(t, err, `Put /system/health/v1/node/diagnostics/bundle-0: unsupported protocol scheme ""`)
	assert.Nil(t, bundle)

	bundle, err = client.Status(context.TODO(), ``, "bundle-0")
	assert.EqualError(t, err, `Get /system/health/v1/node/diagnostics/bundle-0: unsupported protocol scheme ""`)
	assert.Nil(t, bundle)

	err = client.GetFile(context.TODO(), ``, "bundle-0", "")
	assert.EqualError(t, err, `Get /system/health/v1/node/diagnostics/bundle-0/file: unsupported protocol scheme ""`)
}

func TestGetStatusReturnsErrorWhenResponseIsMalformed(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
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
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0/file", r.URL.Path)
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

func TestGetFileReturnsErrorWhenCouldNotCreateAFile(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0/file", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	name, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(name)

	err = client.GetFile(context.TODO(), testServer.URL, "bundle-0", name)
	assert.Contains(t, err.Error(), "could not create a file")
}

func TestList(t *testing.T) {
	expectedResponse := []*Bundle{
		{
			ID:     "bundle-0",
			Status: Done,
		},
		{
			ID:     "bundle-1",
			Status: InProgress,
		},
	}
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		response := jsonMarshal(expectedResponse)
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	}))

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	bundles, err := client.List(context.TODO(), testServer.URL)
	require.NoError(t, err)

	assert.EqualValues(t, expectedResponse, bundles)
}

func TestListWhenResponseEmpty(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.WriteHeader(http.StatusOK)
		w.Write(jsonMarshal([]*Bundle{}))
	}))

	testClient := testServer.Client()

	client := DiagnosticsClient{
		client: testClient,
	}

	bundles, err := client.List(context.TODO(), testServer.URL)
	require.NoError(t, err)
	assert.Len(t, bundles, 0)
}

func TestDelete(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)

		w.WriteHeader(http.StatusOK)
	}))

	client := DiagnosticsClient{
		client: testServer.Client(),
	}

	err := client.Delete(context.TODO(), testServer.URL, "bundle-0")
	require.NoError(t, err)
}

func TestDeleteWhenBundleNotFound(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)

		w.WriteHeader(http.StatusNotFound)
	}))

	client := DiagnosticsClient{
		client: testServer.Client(),
	}

	err := client.Delete(context.TODO(), testServer.URL, "bundle-0")
	assert.EqualError(t, err, "bundle bundle-0 not found")
}

func TestDeleteWhenBundleAlreadyDeleted(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)

		w.WriteHeader(http.StatusNotModified)
	}))

	client := DiagnosticsClient{
		client: testServer.Client(),
	}

	err := client.Delete(context.TODO(), testServer.URL, "bundle-0")
	assert.EqualError(t, err, "bundle bundle-0 canceled or already deleted")
}

func TestDeleteWhenBundleUnreadable(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/system/health/v1/node/diagnostics/bundle-0", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)

		w.WriteHeader(http.StatusInternalServerError)
	}))

	client := DiagnosticsClient{
		client: testServer.Client(),
	}

	err := client.Delete(context.TODO(), testServer.URL, "bundle-0")
	assert.IsType(t, &DiagnosticsBundleUnreadableError{}, err)
}
