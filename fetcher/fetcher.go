package fetcher

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// EndpointRequest is a struct passed to Fetcher with information about URL to be fetched
type EndpointRequest struct {
	URL      string
	Node     dcos.Node
	FileName string
	Optional bool
}

// StatusUpdate is an update message published by Fetcher when EndpointRequest is done. If error occurred	 during
// fetch then Error field is not nil.
type StatusUpdate struct {
	URL   string
	Error error
}

// BulkResponse is a message published when Fetcher finish it job due to cancelled context or closed endpoints chanel
type BulkResponse struct {
	ZipFilePath string
	Error       error
}

// Fetcher is a struct responsible for fetching nodes endpoints
type Fetcher struct {
	file          *os.File
	client        *http.Client
	endpoints     <-chan EndpointRequest
	statusUpdate  chan<- StatusUpdate
	results       chan<- BulkResponse
	prometheusVec prometheus.ObserverVec
}

// New creates new Fetcher. Fetcher needs to be started with Run()
func New(
	tempdir string,
	client *http.Client,
	input <-chan EndpointRequest,
	statusUpdate chan<- StatusUpdate,
	output chan<- BulkResponse,
	prometheusVec prometheus.ObserverVec,
) (*Fetcher, error) {
	f, err := ioutil.TempFile(tempdir, "")
	if err != nil {
		return nil, fmt.Errorf("could not create temp zip file in %s: %s", tempdir, err)
	}

	fetcher := &Fetcher{f, client, input, statusUpdate, output, prometheusVec}

	return fetcher, nil
}

// Run starts fetcher. This method should be run as a goroutine
func (f *Fetcher) Run(ctx context.Context) {
	zipWriter := zip.NewWriter(f.file)

	f.workOffRequests(ctx, zipWriter)

	filename := f.file.Name()

	err := zipWriter.Close()
	if err != nil {
		err = fmt.Errorf("could not close zip writer %s: %s", f.file.Name(), err)
	}
	if e := f.file.Close(); err != nil {
		err = fmt.Errorf("could not close zip file %s: %s: %s", f.file.Name(), e, err)
	}
	f.results <- BulkResponse{
		ZipFilePath: filename,
		Error:       err,
	}
}

func (f *Fetcher) workOffRequests(ctx context.Context, zipWriter *zip.Writer) {
	for {
		select {
		case <-ctx.Done():
			return
		case in, ok := <-f.endpoints:
			if !ok {
				return
			}
			err := f.getDataToZip(ctx, in, zipWriter)
			select {
			case <-ctx.Done():
				return
			default:
				f.statusUpdate <- StatusUpdate{
					URL:   in.URL,
					Error: err,
				}
			}
		}
	}
}

func (f *Fetcher) getDataToZip(ctx context.Context, r EndpointRequest, zipWriter *zip.Writer) error {
	start := time.Now()

	resp, err := get(ctx, f.client, r.URL)
	if err != nil {
		if !r.Optional {
			return fmt.Errorf("could not get from url %s: %s", r.URL, err)
		}
		logrus.WithError(err).Infof("Failed to fetch OPTIONAL URL %s", r.URL)
		return nil
	}

	duration := time.Since(start)
	f.prometheusVec.WithLabelValues(resp.Request.URL.Path, strconv.Itoa(resp.StatusCode)).Observe(duration.Seconds())

	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") == "gzip" {
		r.FileName += ".gz"
	}

	filename := filepath.Join(r.Node.IP+"_"+r.Node.Role, r.FileName)
	zipFile, err := zipWriter.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create a %s in the zip: %s", filename, err)
	}
	if _, err := io.Copy(zipFile, resp.Body); err != nil {
		return fmt.Errorf("could not copy data to zip: %s", err)
	}

	return nil
}

func get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	logrus.Debugf("Using URL %s to collect a log", url)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create a new HTTP request: %s", err)
	}
	request = request.WithContext(ctx)
	request.Header.Add("Accept-Encoding", "gzip")

	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not fetch url %s: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)

		var body []byte
		if resp.Header.Get("Content-Encoding") == "gzip" {
			r, e := gzip.NewReader(resp.Body)
			if e != nil {
				return nil, fmt.Errorf("%s Could not read compressed body: %s", errMsg, e)
			}
			body, e = ioutil.ReadAll(r)
			if e != nil {
				return nil, fmt.Errorf("%s Could not uncompress body: %s", errMsg, e)
			}
		} else {
			body, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("%s Could not read body: %s", errMsg, err)
			}
		}

		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp, err
}
