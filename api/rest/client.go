package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

// Client is an interface that can talk with dcos-diagnostics REST API and manipulate remote bundles
type Client interface {
	// Create ask node url to start bundle creation process with given id
	Create(ctx context.Context, node string, id string) (*Bundle, error)
	// Status returns status of bundle with given id on node at the given url
	Status(ctx context.Context, node string, id string) (*Bundle, error)
	// GetFile downloads bundle file of bundle with given id from node at the given
	// url and save it to local filesystem under given path.
	// Returns an error if there were a problem.
	GetFile(ctx context.Context, node string, id string, path string) (err error)
}

type DiagnosticsClient struct {
	client *http.Client
}

// NewDiagnosticsClient constructs a diagnostics client
func NewDiagnosticsClient() DiagnosticsClient {
	return DiagnosticsClient{
		client: &http.Client{},
	}
}

func (d DiagnosticsClient) Create(ctx context.Context, node string, id string) (*Bundle, error) {
	url := remoteURL(node, id)

	logrus.WithField("bundle", id).WithField("url", url).Info("sending bundle creation request")

	type payload struct {
		Type Type `json:"type"`
	}

	body := jsonMarshal(payload{
		Type: Local,
	})

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

func (d DiagnosticsClient) Status(ctx context.Context, node string, id string) (*Bundle, error) {
	url := remoteURL(node, id)

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

func (d DiagnosticsClient) GetFile(ctx context.Context, node string, id string, path string) error {
	url := fmt.Sprintf("%s/file", remoteURL(node, id))

	logrus.WithField("bundle", id).WithField("url", url).Info("downloading local bundle from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received unexpected status %d", resp.StatusCode)
	}

	destinationFile, err := os.Create(path)
	if err != nil {
		return nil
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, resp.Body)
	if err != nil {
		return err
	}

	// return the full path to the created file
	return nil
}

func remoteURL(node string, id string) string {
	url := fmt.Sprintf("%s/system/health/v1/diagnostics/%s", node, id)
	return url
}
