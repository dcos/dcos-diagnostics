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

const bundlesEndpoint = "/system/health/v1/node/diagnostics"

// Client is an interface that can talk with dcos-diagnostics REST API and manipulate remote bundles
type Client interface {
	// CreateBundle requests the given node to start a bundle creation process with that is identified by the given ID
	CreateBundle(ctx context.Context, node string, ID string) (*Bundle, error)
	// Status returns the status of the bundle with the given ID on the given node
	Status(ctx context.Context, node string, ID string) (*Bundle, error)
	// GetFile downloads the bundle file of the bundle with the given ID from the node
	// url and save it to local filesystem under given path.
	// Returns an error if there were a problem.
	GetFile(ctx context.Context, node string, ID string, path string) (err error)
	// List will get the list of available bundles on the given node
	List(ctx context.Context, node string) ([]*Bundle, error)
	// Delete will delete the bundle with the given id from the given node
	Delete(ctx context.Context, node string, ID string) error
}

type DiagnosticsClient struct {
	client *http.Client
}

// NewDiagnosticsClient constructs a diagnostics client
func NewDiagnosticsClient(client *http.Client) DiagnosticsClient {
	return DiagnosticsClient{
		client: client,
	}
}

func (d DiagnosticsClient) CreateBundle(ctx context.Context, node string, ID string) (*Bundle, error) {
	url := remoteURL(node, ID)

	logrus.WithField("ID", ID).WithField("url", url).Debug("sending bundle creation request")

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

	err = handleErrorCode(resp, url, ID)
	if err != nil {
		return nil, err
	}

	bundle := &Bundle{}
	err = json.NewDecoder(resp.Body).Decode(bundle)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (d DiagnosticsClient) Status(ctx context.Context, node string, ID string) (*Bundle, error) {
	url := remoteURL(node, ID)

	logrus.WithField("ID", ID).WithField("url", url).Debug("checking status of bundle")

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

	bundle := &Bundle{}

	err = handleErrorCode(resp, url, ID)
	if err != nil {
		return nil, err
	}

	err = json.NewDecoder(resp.Body).Decode(bundle)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (d DiagnosticsClient) GetFile(ctx context.Context, node string, ID string, path string) error {
	url := fmt.Sprintf("%s/file", remoteURL(node, ID))

	logrus.WithField("ID", ID).WithField("url", url).Debug("downloading local bundle from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = handleErrorCode(resp, url, ID)
	if err != nil {
		return err
	}

	destinationFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create a file: %s", err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func (d DiagnosticsClient) List(ctx context.Context, node string) ([]*Bundle, error) {
	url := fmt.Sprintf("%s%s", node, bundlesEndpoint)

	logrus.WithField("node", node).Debug("getting list of bundles from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// there are no expected error messages that could come from this so having
	// a null id should be fine as the only case this should hit is the default
	// unexpected status code case.
	err = handleErrorCode(resp, url, "")
	if err != nil {
		return nil, err
	}

	bundles := []*Bundle{}
	err = json.NewDecoder(resp.Body).Decode(&bundles)
	if err != nil {
		return nil, err
	}

	return bundles, nil
}

func (d DiagnosticsClient) Delete(ctx context.Context, node string, id string) error {
	url := remoteURL(node, id)

	logrus.WithField("node", node).WithField("ID", id).Debug("deleting bundle from node")

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return handleErrorCode(resp, url, id)
}

func handleErrorCode(resp *http.Response, url string, bundleID string) error {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return &DiagnosticsBundleNotFoundError{id: bundleID}
	case resp.StatusCode == http.StatusInternalServerError:
		return &DiagnosticsBundleUnreadableError{id: bundleID}
	case resp.StatusCode != http.StatusOK:
		body := make([]byte, 100)
		resp.Body.Read(body)
		return fmt.Errorf("received unexpected status code [%d] from %s: %s", resp.StatusCode, url, string(body))
	}
	return nil
}

func remoteURL(node string, ID string) string {
	url := fmt.Sprintf("%s%s/%s", node, bundlesEndpoint, ID)
	return url
}
