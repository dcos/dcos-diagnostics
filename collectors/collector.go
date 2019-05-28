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

// NameOptional is a struct that should be inherited by Collectors to prevent name clash with field and interface functions
type NameOptional struct {
	Name     string
	Optional bool
}

// CmdCollector is a struct implementing Collector interface. It collects command output for given command configured with Cmd field
type CmdCollector struct {
	NameOptional
	Cmd []string
}

func (c CmdCollector) Name() string {
	return c.NameOptional.Name
}

func (c CmdCollector) Optional() bool {
	return c.NameOptional.Optional
}

func (c CmdCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, c.Cmd[0], c.Cmd[1:]...)
	output, err := cmd.CombinedOutput()
	return ioutil.NopCloser(bytes.NewReader(output)), err
}

// TODO(janisz): Make use of this code instead of calling dcos-diagnostics for units data
// See: https://github.com/dcos/dcos-diagnostics/blob/3734e2e03644449500427fb916289c4007dc5106/api/providers.go#L96-L103
//type SystemdCollector struct {
//	N        string
//	UnitName string
//	Duration time.Duration
//}
//
//func (c SystemdCollector) Name() string {
//	return c.N
//}
//
//func (c SystemdCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
//	return units.ReadJournalOutputSince(ctx, c.UnitName, c.Duration.String())
//}

// EndpointCollector is a struct implementing Collector interface. It collects HTTP response for given URL
type EndpointCollector struct {
	NameOptional
	Client *http.Client
	URL    string
}

func (c EndpointCollector) Name() string {
	return c.NameOptional.Name
}

func (c EndpointCollector) Optional() bool {
	return c.NameOptional.Optional
}

func (c EndpointCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	url := c.URL
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create a new HTTP request: %s", err)
	}
	request = request.WithContext(ctx)

	resp, err := c.Client.Do(request)
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

type FileCollector struct {
	NameOptional
	FilePath string
}

func (c FileCollector) Name() string {
	return c.NameOptional.Name
}

func (c FileCollector) Optional() bool {
	return c.NameOptional.Optional
}

func (c FileCollector) Collect(ctx context.Context) (io.ReadCloser, error) {
	r, err := os.Open(c.FilePath)
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
