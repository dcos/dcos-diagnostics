package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

// Client is an interface that can talk with dcos-diagnostics REST API and manipulate remote bundles
type Client interface {
	// Create ask node url to start bundle creation process with given id
	Create(ctx context.Context, node string, id string) (*Bundle, error)
	// Status returns status of bundle with given id on node at the given url
	Status(ctx context.Context, node string, id string) (*Bundle, error)
	// GetFile downloads bundle file of bundle with given id from node at the given
	// url and save it to local filesystem. It returns path where file is stored or
	// error if there were a problem.
	GetFile(ctx context.Context, node string, id string) (path string, err error)
}

type bundleStatus struct {
	ID   string
	node node
	done bool
	err  error
}

type DiagnosticsClient struct {
	client   *http.Client
	forceTLS bool
}

func newDiagnosticsClient() *DiagnosticsClient {
	return &DiagnosticsClient{}
}

func (d *DiagnosticsClient) Create(ctx context.Context, node string, id string) (*Bundle, error) {
	url := fmt.Sprintf("%s/system/health/v1/diagnostics/%s", node, id)

	logrus.WithField("bundle", id).WithField("url", url).Info("sending bundle creation request")

	type payload struct {
		Type string `json:"type"`
	}

	body, err := json.Marshal(payload{
		Type: "LOCAL",
	})
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	request.WithContext(ctx)

	resp, err := d.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status code received: %d", resp.StatusCode)
	}

	var bundle Bundle
	err = json.NewDecoder(resp.Body).Decode(&bundle)
	if err != nil {
		return nil, err
	}

	return &bundle, nil
}

func (d *DiagnosticsClient) Status(ctx context.Context, node string, id string) (*Bundle, error) {
	url := fmt.Sprintf("%s/system/health/v1/diagnostics/%s", node, id)

	logrus.WithField("bundle", id).WithField("url", url).Info("checking status of bundle")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	request.WithContext(ctx)

	resp, err := d.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bundle Bundle

	if resp.StatusCode == http.StatusNotFound {
		bundle.Status = Unknown
		return &bundle, nil
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received unexpected status code from %s: %d", url, resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&bundle)
	if err != nil {
		return nil, err
	}

	return &bundle, nil
}

func (d *DiagnosticsClient) GetFile(ctx context.Context, node string, id string) (string, error) {
	url := fmt.Sprintf("%s/system/health/v1/diagnostics/%s/file", node, id)

	logrus.WithField("bundle", id).WithField("url", url).Info("downloading local bundle from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("bundle %s not known to node %s", id, url)
	} else if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received unexpected status %d", resp.StatusCode)
	}

	// the '*' will be swapped out with a random string by ioutil.TempFile
	destinationName := fmt.Sprintf("bundle-%s-*.zip", id)
	// This will use the default temp directory from os.TempDir
	destinationFile, err := ioutil.TempFile("", destinationName)
	if err != nil {
		return "", nil
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, resp.Body)
	if err != nil {
		return "", err
	}

	return destinationName, nil
}
