package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/dcos/dcos-diagnostics/config"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

type fetcher struct {
	Cfg        *config.Config
	StatusChan chan<- statusUpdate
	Client     *http.Client

	failed bool
}

type statusUpdate struct {
	incPercentage float32
	error         error
	msg           string
}

func (f *fetcher) incJobProgressPercentage(percentage float32) {
	f.StatusChan <- statusUpdate{incPercentage: percentage}
}

func (f *fetcher) appendError(e error) {
	f.failed = true
	f.StatusChan <- statusUpdate{error: e}
}

func (f *fetcher) setStatus(s string) {
	f.StatusChan <- statusUpdate{msg: s}
}

func (f *fetcher) collectDataFromNodes(ctx context.Context, nodes []dcos.Node, summaryReport *bytes.Buffer,
	summaryErrorsReport *bytes.Buffer, zipWriter *zip.Writer) error {
	// we already checked for nodes length, we should not get division by zero error at this point.
	percentPerNode := 100.0 / float32(len(nodes))
	for _, node := range nodes {
		updateSummaryReport("START collecting logs", node, "", summaryReport)
		endpoints, err := f.getNodeEndpoints(ctx, node)
		if err != nil {
			f.logError(err, node, summaryErrorsReport)
			f.incJobProgressPercentage(percentPerNode)
		}

		// add http endpoints
		err = f.getHTTPAddToZip(ctx, node, endpoints, zipWriter, summaryErrorsReport, summaryReport, percentPerNode)
		if err != nil {
			f.appendError(err)

			// handle job cancel error
			if _, ok := err.(diagnosticsJobCanceledError); ok {
				return fmt.Errorf("could not add diagnostics to zip file: %s", err)
			}

			logrus.WithError(err).Errorf("Could not add a log to a bundle: %s", err)
			updateSummaryReport(err.Error(), node, err.Error(), summaryErrorsReport)
		}
		updateSummaryReport("STOP collecting logs", node, "", summaryReport)
	}
	if f.failed {
		return fmt.Errorf("diagnostics job failed")
	}
	return nil
}

func (f *fetcher) getNodeEndpoints(ctx context.Context, node dcos.Node) (endpoints map[string]string, e error) {
	port, err := getPullPortByRole(f.Cfg, node.Role)
	if err != nil {
		e = fmt.Errorf("used incorrect role: %s", err)
		return nil, e
	}
	url := fmt.Sprintf("http://%s:%d%s/logs", node.IP, port, baseRoute)

	c, cancelFunc := context.WithTimeout(ctx, time.Second*3)
	defer cancelFunc()
	resp, err := get(c, f.Client, url)

	if err != nil {
		return nil, fmt.Errorf("could not get endpoints %s", err)
	}

	defer closeAndLogErr(resp.Body)
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil || resp.StatusCode != http.StatusOK {
		e := fmt.Errorf("could not get a list of logs, url: %s, status code %d: %s", url, resp.StatusCode, err)
		return nil, e
	}
	if err = json.Unmarshal(body, &endpoints); err != nil {
		e := fmt.Errorf("could not unmarshal a list of logs from %s: %s", url, err)
		return nil, e
	}
	if len(endpoints) == 0 {
		e := fmt.Errorf("no endpoints found, url %s", url)
		return nil, e
	}
	return endpoints, nil
}

// fetch an HTTP endpoint and append the output to a zip file.
func (f *fetcher) getHTTPAddToZip(ctx context.Context, node dcos.Node, endpoints map[string]string, zipWriter *zip.Writer,
	summaryErrorsReport, summaryReport *bytes.Buffer, percentPerNode float32) error {
	percentPerURL := percentPerNode / float32(len(endpoints))

	for fileName, httpEndpoint := range endpoints {
		select {
		case <-ctx.Done():
			updateSummaryReport("Job canceled", node, "", summaryErrorsReport)
			updateSummaryReport("Job canceled", node, "", summaryReport)
			return diagnosticsJobCanceledError{
				msg: "Job canceled",
			}
		default:
			logrus.Debugf("GET %s%s", node.IP, httpEndpoint)
		}

		status := "GET " + node.IP + httpEndpoint
		updateSummaryReport("START "+status, node, "", summaryReport)
		e := f.getDataToZip(ctx, node, httpEndpoint, fileName, zipWriter)
		updateSummaryReport("STOP "+status, node, "", summaryReport)
		f.setStatus(status)
		if e != nil {
			f.logError(e, node, summaryErrorsReport)
		}
		f.incJobProgressPercentage(percentPerURL)
	}
	return nil
}

func (f *fetcher) getDataToZip(ctx context.Context, node dcos.Node, httpEndpoint string, fileName string, zipWriter *zip.Writer) error {
	fullURL, err := util.UseTLSScheme("http://"+node.IP+httpEndpoint, f.Cfg.FlagForceTLS)
	if err != nil {
		e := fmt.Errorf("could not read force-tls flag: %s", err)
		return e
	}
	resp, err := get(ctx, f.Client, fullURL)
	if err != nil {
		e := fmt.Errorf("could not get from url %s: %s", fullURL, err)
		return e
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		fileName += ".gz"
	}
	// put all logs in a `ip_role` folder
	zipFile, err := zipWriter.Create(filepath.Join(node.IP+"_"+node.Role, fileName))
	defer closeAndLogErr(resp.Body)
	if err != nil {
		e := fmt.Errorf("could not add %s to a zip archive: %s", fileName, err)
		return e
	}
	if _, err := io.Copy(zipFile, resp.Body); err != nil {
		return fmt.Errorf("could not copy response the zip: %s", err)
	}
	return nil
}

func (f *fetcher) logError(e error, node dcos.Node, summaryErrorsReport *bytes.Buffer) {
	f.appendError(e)
	logrus.Error(e)
	updateSummaryReport(e.Error(), node, e.Error(), summaryErrorsReport)
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
		closeAndLogErr(resp.Body)
		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)
		if e != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, e)
		}
		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp, err
}

func closeAndLogErr(closable io.Closer) {
	if err := closable.Close(); err != nil {
		logrus.WithError(err).Warn("Could not closeAndLogErr")
	}
}
