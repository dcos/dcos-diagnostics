package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

func (j *DiagnosticsJob) collectDataFromNodes(ctx context.Context, nodes []dcos.Node, summaryReport *bytes.Buffer,
	summaryErrorsReport *bytes.Buffer, zipWriter *zip.Writer) error {
	// we already checked for nodes length, we should not get division by zero error at this point.
	percentPerNode := 100.0 / float32(len(nodes))
	for _, node := range nodes {
		updateSummaryReport("START collecting logs", node, "", summaryReport)
		endpoints, err := j.getNodeEndpoints(node)
		if err != nil {
			j.logError(err, node, summaryErrorsReport)
			j.incJobProgressPercentage(percentPerNode)
		}

		// add http endpoints
		err = j.getHTTPAddToZip(ctx, node, endpoints, zipWriter, summaryErrorsReport, summaryReport, percentPerNode)
		if err != nil {
			j.appendError(err)

			// handle job cancel error
			if _, ok := err.(diagnosticsJobCanceledError); ok {
				err := fmt.Errorf("could not add diagnostics to zip file: %s", err)
				return err
			}

			logrus.WithError(err).Errorf("Could not add a log to a bundle: %s", err)
			updateSummaryReport(err.Error(), node, err.Error(), summaryErrorsReport)
		}
		updateSummaryReport("STOP collecting logs", node, "", summaryReport)
	}
	errors := j.getErrors()
	if len(errors) != 0 {
		return fmt.Errorf("diagnostics job failed: %v", j.errors)
	}
	return nil
}

// fetch an HTTP endpoint and append the output to a zip file.
func (j *DiagnosticsJob) getHTTPAddToZip(ctx context.Context, node dcos.Node, endpoints map[string]string, zipWriter *zip.Writer,
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
		e := j.getDataToZip(ctx, node, httpEndpoint, fileName, zipWriter)
		updateSummaryReport("STOP "+status, node, "", summaryReport)
		j.setStatus(status)
		if e != nil {
			j.logError(e, node, summaryErrorsReport)
		}
		j.incJobProgressPercentage(percentPerURL)
	}
	return nil
}

func (j *DiagnosticsJob) getDataToZip(ctx context.Context, node dcos.Node, httpEndpoint string, fileName string, zipWriter *zip.Writer) error {
	fullURL, err := util.UseTLSScheme("http://"+node.IP+httpEndpoint, j.Cfg.FlagForceTLS)
	if err != nil {
		e := fmt.Errorf("could not read force-tls flag: %s", err)
		return e
	}
	resp, err := get(ctx, j.client, fullURL)
	if err != nil {
		e := fmt.Errorf("could not get from url %s: %s", fullURL, err)
		return e
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		fileName += ".gz"
	}
	// put all logs in a `ip_role` folder
	zipFile, err := zipWriter.Create(filepath.Join(node.IP+"_"+node.Role, fileName))
	defer resp.Body.Close()
	if err != nil {
		e := fmt.Errorf("could not add %s to a zip archive: %s", fileName, err)
		return e
	}
	io.Copy(zipFile, resp.Body)
	return nil
}

func (j *DiagnosticsJob) logError(e error, node dcos.Node, summaryErrorsReport *bytes.Buffer) {
	j.appendError(e)
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
		resp.Body.Close()
		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)
		if e != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, e)
		}
		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp, err
}
