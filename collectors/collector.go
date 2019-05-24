package collectors

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

	"github.com/dcos/dcos-diagnostics/units"
)

// Collector is the interface to abstract data collection from different sources
type Collector interface {
	// Name returns the name of this collector
	Name() string
	// Collect returns collected data
	Collect(ctx context.Context) (io.ReadCloser, error)
}

type CmdCollector struct {
	name string
	cmd  []string
}

func (c CmdCollector) Name() string {
	return c.name
}

func (c CmdCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, c.cmd[0], c.cmd[1:]...)
	output, err := cmd.CombinedOutput()
	return ioutil.NopCloser(bytes.NewReader(output)), err
}

type SystemdCollector struct {
	name     string
	unitName string
	duration time.Duration
}

func (c SystemdCollector) Name() string {
	return c.name
}

func (c SystemdCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	return units.ReadJournalOutputSince(ctx, c.unitName, c.duration.String())
}

type EndpointCollector struct {
	name   string
	client *http.Client
	url    string
}

func (c EndpointCollector) Name() string {
	return c.name
}

func (c EndpointCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
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

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, err)
		}

		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp.Body, err
}

type FileCollector struct {
	name     string
	filePath string
}

func (c FileCollector) Name() string {
	return c.name
}

func (c FileCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
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
