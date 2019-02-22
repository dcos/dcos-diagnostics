package fetcher

import (
	"archive/zip"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_FetcherReturnErrorWhenCantCreateZip(t *testing.T) {
	err := Fetcher(context.TODO(), "not_existing_dir", nil, nil, nil, nil)
	assert.Contains(t, err.Error(), "could not create temp zip file in not_existing_dir")
}

func Test_FetcherReturnEmptyZipOnClosedContext(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancelFunc()

	output := make(chan FetchBulkResponse)

	err := Fetcher(ctx, "", nil, nil, nil, output)
	assert.NoError(t, err)

	zipfile := <-output

	z, err := zip.OpenReader(zipfile.ZipFilePath)
	assert.NoError(t, err)
	assert.Empty(t, z.File)

}

func Test_FetcherShouldSentUpdateAfterFetchingAnEndpoint(t *testing.T) {
	input := make(chan EndpointFetchRequest)
	statusUpdate := make(chan FetchStatusUpdate)
	output := make(chan FetchBulkResponse)

	server, _ := stubServer("/ping", "pong")
	host := "http://" + server.URL[7:]
	defer server.Close()

	err := Fetcher(context.TODO(), "", http.DefaultClient, input, statusUpdate, output)
	assert.NoError(t, err)

	input <- EndpointFetchRequest{
		URL:      host + "/ping",
		Node:     dcos.Node{IP: "127.0.0.1", Role: dcos.AgentRole},
		FileName: "ping_file",
	}

	assert.Equal(t, FetchStatusUpdate{URL: host + "/ping"}, <-statusUpdate)

	input <- EndpointFetchRequest{
		URL:      host + "/error",
		Node:     dcos.Node{IP: "127.0.0.2", Role: dcos.MasterRole},
		FileName: "error_file",
	}

	status := <-statusUpdate
	assert.Equal(t, host+"/error", status.URL)
	assert.Contains(t, status.Error.Error(), "Return code 404. Body: 404 page not found")

	close(input)

	zipfile := <-output

	z, err := zip.OpenReader(zipfile.ZipFilePath)
	require.NoError(t, err)
	assert.Len(t, z.File, 1)

	rc, err := z.File[0].Open()
	require.NoError(t, err)

	body, err := ioutil.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))
}

// http://keighl.com/post/mocking-http-responses-in-golang/
func stubServer(uri string, body string) (*httptest.Server, *http.Transport) {
	return mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() == uri {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(body))
		} else {
			http.NotFound(w, r)
		}
	})
}

func mockServer(handle func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *http.Transport) {
	server := httptest.NewServer(http.HandlerFunc(handle))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return server, transport
}
