package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Collector is the interface to abstract data collection from different sources
type Collector interface {
	// Name returns the Name of this collector
	Name() string
	// Optional returns true if Collector is not mandatory and failures should be ignored
	Optional() bool
	// Collect returns collected data
	Collect(ctx context.Context) (io.ReadCloser, error)
}

// Cmd is a struct implementing Collector interface. It collects command output for given command configured with Cmd field
type Cmd struct {
	name     string
	optional bool
	cmd      []string
}

func NewCmd(name string, optional bool, cmd []string) *Cmd {
	return &Cmd{
		name:     name,
		optional: optional,
		cmd:      cmd,
	}
}

func (c Cmd) Name() string {
	return c.name
}

func (c Cmd) Optional() bool {
	return c.optional
}

func (c Cmd) Collect(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, c.cmd[0], c.cmd[1:]...)
	output, err := cmd.CombinedOutput()
	return ioutil.NopCloser(bytes.NewReader(output)), err
}

// TODO(janisz): Make use of this code instead of calling dcos-diagnostics for units data https://jira.mesosphere.com/browse/DCOS_OSS-5223
// See: https://github.com/dcos/dcos-diagnostics/blob/3734e2e03644449500427fb916289c4007dc5106/api/providers.go#L96-L103
//type Systemd struct {
//	name        string
//	unitName string
//	duration time.Duration
//}
//
//func (c Systemd) Name() string {
//	return c.name
//}
//
//func (c Systemd) Collect(ctx context.Context) (io.ReadCloser, error) {
//	return units.ReadJournalOutputSince(ctx, c.unitName, c.duration.String())
//}

// Endpoint is a struct implementing Collector interface. It collects HTTP response for given url
type Endpoint struct {
	name     string
	optional bool
	client   *http.Client
	url      string
}

func NewEndpoint(name string, optional bool, url string, client *http.Client) *Endpoint {
	return &Endpoint{
		name:     name,
		optional: optional,
		url:      url,
		client:   client,
	}
}

func (c Endpoint) Name() string {
	return c.name
}

func (c Endpoint) Optional() bool {
	return c.optional
}

func (c Endpoint) Collect(ctx context.Context) (io.ReadCloser, error) {
	url := c.url
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create a new HTTP request: %s", err)
	}
	request = request.WithContext(ctx)

	resp, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not fetch url %s: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)

		body, e := ioutil.ReadAll(resp.Body)
		if e != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, e)
		}

		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp.Body, err
}

type File struct {
	name     string
	optional bool
	filePath string
}

func NewFile(name string, optional bool, filePath string) *File {
	return &File{
		name:     name,
		optional: optional,
		filePath: filePath,
	}
}

func (c File) Name() string {
	return c.name
}

func (c File) Optional() bool {
	return c.optional
}

func (c File) Collect(ctx context.Context) (io.ReadCloser, error) {
	r, err := os.Open(c.filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %s", c.Name(), err)
	}
	return ReadCloser(ctx, r), nil
}

// ReadCloser wraps an io.ReadCloser with one that checks ctx.Done() on each Read call.
//
// If ctx has a deadline and if r has a `SetReadDeadline(time.Time) error` method,
// then it is called with the deadline.
//
// Source : https://gist.github.com/dchapes/6c992bf3e943934462509338cd213e99
func ReadCloser(ctx context.Context, r io.ReadCloser) io.ReadCloser {
	if deadline, ok := ctx.Deadline(); ok {
		type deadliner interface {
			SetReadDeadline(time.Time) error
		}
		if d, ok := r.(deadliner); ok {
			_ = d.SetReadDeadline(deadline)
		}
	}
	return reader{ctx, r}
}

type reader struct {
	ctx context.Context
	r   io.ReadCloser
}

func (r reader) Read(p []byte) (n int, err error) {
	if err = r.ctx.Err(); err != nil {
		return
	}
	if n, err = r.r.Read(p); err != nil {
		return
	}
	err = r.ctx.Err()
	return
}

func (r reader) Close() error {
	return r.r.Close()
}
