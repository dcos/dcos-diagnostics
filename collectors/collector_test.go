package collectors

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdCollectorIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(CmdCollector))
}

func TestCmdCollector_Name(t *testing.T) {
	assert.Equal(t, "test", CmdCollector{name: "test"}.Name())
}

func TestCmdCollector_Collect(t *testing.T) {
	c := CmdCollector{name: "echo", cmd: []string{"echo", "OK"}}
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, "OK\n", string(raw))

	c = CmdCollector{name: "unknown", cmd: []string{"unknown", "command"}}
	r, err = c.Collect(context.TODO())
	assert.EqualError(t, err, fmt.Sprintf("exec: \"unknown\": executable file not found in $PATH"))

	raw, err = ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Empty(t, string(raw))
}
func TestSystemdCollectorIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(SystemdCollector))
}

func TestSystemdCollector_Name(t *testing.T) {
	assert.Equal(t, "test", SystemdCollector{name: "test"}.Name())
}

func TestEndpointCollectorIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(EndpointCollector))
}

func TestEndpointCollector_Name(t *testing.T) {
	assert.Equal(t, "test", EndpointCollector{name: "test"}.Name())
}

func TestEndpointCollector_Collect(t *testing.T) {
	server, _ := stubServer("/ping", "OK")
	defer server.Close()

	c := EndpointCollector{name: "ping", url: server.URL + "/ping", client: http.DefaultClient}
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(raw))

	c = EndpointCollector{name: "test", url: server.URL + "/test", client: http.DefaultClient}
	r, err = c.Collect(context.TODO())

	assert.EqualError(t, err, fmt.Sprintf("unable to fetch %s. Return code 404. Body: 404 page not found\n", server.URL+"/test"))
}

func TestEndpointCollector_CollectShouldReturnErrorWhen404(t *testing.T) {
	server, _ := stubServer("/ping", "OK")
	defer server.Close()

	c := EndpointCollector{name: "test", url: server.URL + "/test", client: http.DefaultClient}
	r, err := c.Collect(context.TODO())

	assert.Nil(t, r)
	assert.EqualError(t, err, fmt.Sprintf("unable to fetch %s. Return code 404. Body: 404 page not found\n", server.URL+"/test"))
}

func TestEndpointCollector_CollectShouldReturnErrorWhenNoServer(t *testing.T) {
	http.DefaultClient.Timeout = time.Millisecond
	c := EndpointCollector{name: "test", url: "http://192.0.2.0/test", client: http.DefaultClient}
	r, err := c.Collect(context.TODO())

	assert.Nil(t, r)
	assert.EqualError(t, err, "could not fetch url http://192.0.2.0/test: Get http://192.0.2.0/test: net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)")
}

func TestEndpointCollector_CollectShouldReturnErrorWhenInvalidURL(t *testing.T) {
	http.DefaultClient.Timeout = time.Millisecond
	c := EndpointCollector{name: "test", url: "invalid url", client: http.DefaultClient}
	r, err := c.Collect(context.TODO())

	assert.Nil(t, r)
	assert.EqualError(t, err, "could not fetch url invalid url: Get invalid%20url: unsupported protocol scheme \"\"")
}

// http://keighl.com/post/mocking-http-responses-in-golang/
func stubServer(uri string, body string) (*httptest.Server, *http.Transport) {
	return mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != uri {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(body))
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

func TestFileCollectorIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(FileCollector))
}

func TestFileCollector_Name(t *testing.T) {
	assert.Equal(t, "test", FileCollector{name: "test"}.Name())
}

func TestFileCollector_Collect(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	_, err = f.Write([]byte("OK"))
	require.NoError(t, err)

	c := FileCollector{
		name:     "test",
		filePath: f.Name(),
	}

	reader, err := c.Collect(context.Background())
	assert.NoError(t, err)

	raw, err := ioutil.ReadAll(reader)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(raw))

	assert.NoError(t, reader.Close())
}

func TestFileCollector_CollectNotExistingFile(t *testing.T) {
	c := FileCollector{
		name:     "test",
		filePath: "not-existing-file",
	}

	reader, err := c.Collect(context.Background())
	assert.Nil(t, reader)
	assert.EqualError(t, err, "could not open test: open not-existing-file: no such file or directory")
}

func TestFileCollector_CollectContextDont(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	_, err = f.Write([]byte("OK"))
	require.NoError(t, err)

	c := FileCollector{
		name:     "test",
		filePath: f.Name(),
	}

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	reader, err := c.Collect(ctx)
	assert.NoError(t, err)

	raw, err := ioutil.ReadAll(reader)

	assert.EqualError(t, err, "context canceled")
	assert.Empty(t, raw)

	assert.NoError(t, reader.Close())
}
