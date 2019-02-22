package fetcher

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/sirupsen/logrus"
)

type EndpointFetchRequest struct {
	URL      string
	Node     dcos.Node
	FileName string
}

type FetchStatusUpdate struct {
	URL   string
	Error error
}

type FetchBulkResponse struct {
	ZipFilePath string
}

func Fetcher(ctx context.Context, tempdir string, client *http.Client, input <-chan EndpointFetchRequest, statusUpdate chan<- FetchStatusUpdate, output chan<- FetchBulkResponse) error {
	f, err := ioutil.TempFile(tempdir, "")
	if err != nil {
		return fmt.Errorf("could not create temp zip file in %s: %s", tempdir, err)
	}

	go fetch(f, ctx, input, err, client, statusUpdate, output)

	return nil
}

func fetch(f *os.File, ctx context.Context, input <-chan EndpointFetchRequest, err error, client *http.Client, statusUpdate chan<- FetchStatusUpdate, output chan<- FetchBulkResponse) {
	zipWriter := zip.NewWriter(f)
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case in, ok := <-input:
			if !ok {
				break LOOP
			}
			err = getDataToZip(ctx, client, in, zipWriter)
			select {
			case <-ctx.Done():
				break LOOP
			case statusUpdate <- FetchStatusUpdate{
				URL:   in.URL,
				Error: err,
			}:
			}
		}

	}
	filename := f.Name()
	if err := zipWriter.Close(); err != nil {
		logrus.WithError(err).Errorf("Could not close zip writer %s", f.Name())
	}
	if err := f.Close(); err != nil {
		logrus.WithError(err).Errorf("Could not close zip file %s", f.Name())
	}
	output <- FetchBulkResponse{
		ZipFilePath: filename,
	}
}

func getDataToZip(ctx context.Context, client *http.Client, r EndpointFetchRequest, zipWriter *zip.Writer) error {
	resp, err := get(ctx, client, r.URL)
	if err != nil {
		return fmt.Errorf("could not get from url %s: %s", r.URL, err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") == "gzip" {
		r.FileName += ".gz"
	}

	zipFile, err := zipWriter.Create(filepath.Join(r.Node.IP+"_"+r.Node.Role, r.FileName))
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
		body, e := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)
		if e != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, e)
		}
		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp, err
}
