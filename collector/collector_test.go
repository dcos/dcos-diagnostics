package collector

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(Cmd))
}

func TestCmd_Name(t *testing.T) {
	assert.Equal(t, "test", NewCmd(
		"test",
		false,
		nil,
	).Name())
}

func TestCmd_Optional(t *testing.T) {
	assert.False(t, NewCmd("test", false, nil).Optional())
	assert.True(t, NewCmd("test", true, nil).Optional())
}

func TestCmd_Collect(t *testing.T) {
	c := NewCmd(
		"echo",
		false,
		[]string{"echo", "OK"},
	)
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, "OK\n", string(raw))

	c = NewCmd(
		"unknown",
		false,
		[]string{"unknown", "command"},
	)
	r, err = c.Collect(context.TODO())
	assert.Contains(t, err.Error(), "exec: \"unknown\": executable file not found")

	raw, err = ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Empty(t, string(raw))
}

func TestSystemdIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(Systemd))
}

func TestSystemd_Name(t *testing.T) {
	assert.Equal(t, "test", NewSystemd(
		"test",
		false,
		"systemd",
		time.Minute,
	).Name())
}

func TestSystemd_Optional(t *testing.T) {
	assert.False(t, NewSystemd("test", false, "systemd", time.Minute).Optional())
	assert.True(t, NewSystemd("test", true, "systemd", time.Minute).Optional())
}

func TestSystemd_Collect(t *testing.T) {
	path, err := exec.LookPath("journalctl")
	if err != nil {
		t.Skipf("SKIPPING: Could not find journalctl: %s", err)
	}
	t.Log("journalctl exists in ", path)

	c := NewSystemd(
		"test",
		false,
		"systemd-journald.service",
		time.Duration(math.MaxInt64),
	)
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Contains(t, string(raw), "Journal started")
}

func TestEndpointIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(Endpoint))
}

func TestEndpoint_Name(t *testing.T) {
	assert.Equal(t, "test", NewEndpoint("test", false, "", nil).Name())
}

func TestEndpoint_Optional(t *testing.T) {
	assert.False(t, NewEndpoint("test", false, "", nil).Optional())
	assert.True(t, NewEndpoint("test", true, "", nil).Optional())
}

func TestEndpoint_Collect(t *testing.T) {
	server, _ := stubServer("/ping", "OK")
	defer server.Close()

	c := NewEndpoint(
		"ping",
		false,
		server.URL+"/ping",
		http.DefaultClient,
	)
	r, err := c.Collect(context.TODO())

	require.NoError(t, err)

	raw, err := ioutil.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(raw))

	c = NewEndpoint(
		"test",
		false,
		server.URL+"/test",
		http.DefaultClient,
	)
	r, err = c.Collect(context.TODO())

	assert.EqualError(t, err, fmt.Sprintf("unable to fetch %s. Return code 404. Body: 404 page not found\n", server.URL+"/test"))
}

func TestEndpoint_CollectShouldReturnErrorWhen404(t *testing.T) {
	server, _ := stubServer("/ping", "OK")
	defer server.Close()

	c := NewEndpoint(
		"test",
		false,
		server.URL+"/test",
		http.DefaultClient,
	)
	r, err := c.Collect(context.TODO())

	assert.Nil(t, r)
	assert.EqualError(t, err, fmt.Sprintf("unable to fetch %s. Return code 404. Body: 404 page not found\n", server.URL+"/test"))
}

func TestEndpoint_CollectShouldReturnErroronTimeout(t *testing.T) {
	http.DefaultClient.Timeout = time.Nanosecond
	c := NewEndpoint(
		"test",
		false,
		"http://192.0.2.0/test",
		http.DefaultClient,
	)
	r, err := c.Collect(context.TODO())

	assert.Nil(t, r)
	assert.EqualError(t, err, "could not fetch url http://192.0.2.0/test: Get http://192.0.2.0/test: net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)")
}

func TestEndpoint_CollectShouldReturnErrorWhenInvalidURL(t *testing.T) {
	http.DefaultClient.Timeout = time.Nanosecond
	c := NewEndpoint(
		"test",
		false,
		"invalid url",
		http.DefaultClient,
	)
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

func TestFileIsCollector(t *testing.T) {
	assert.Implements(t, (*Collector)(nil), new(File))
}

func TestFile_Name(t *testing.T) {
	assert.Equal(t, "test", NewFile(
		"test",
		false,
		"",
	).Name())
}

func TestFile_Optional(t *testing.T) {
	assert.False(t, NewFile("test", false, "").Optional())
	assert.True(t, NewFile("test", true, "").Optional())
}

func TestFile_Collect(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	_, err = f.Write([]byte("OK"))
	require.NoError(t, err)

	c := NewFile(
		"test",
		false,
		f.Name(),
	)

	reader, err := c.Collect(context.Background())
	assert.NoError(t, err)

	raw, err := ioutil.ReadAll(reader)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(raw))

	assert.NoError(t, reader.Close())
}

func TestFile_CollectNotExistingFile(t *testing.T) {
	c := NewFile(
		"test",
		false,
		"not-existing-file",
	)

	reader, err := c.Collect(context.Background())
	assert.Nil(t, reader)
	assert.Contains(t, err.Error(), "could not open test: open not-existing-file:")
}

func TestFile_CollectContextDont(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)

	_, err = f.Write([]byte("OK"))
	require.NoError(t, err)

	c := NewFile(
		"test",
		false,
		f.Name(),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	reader, err := c.Collect(ctx)
	assert.NoError(t, err)

	raw, err := ioutil.ReadAll(reader)

	assert.EqualError(t, err, "context canceled")
	assert.Empty(t, raw)

	assert.NoError(t, reader.Close())
}
