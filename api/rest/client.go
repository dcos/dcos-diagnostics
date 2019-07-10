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
	Delete(ctx context.Context, node string, id string) error
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

	logrus.WithField("ID", ID).WithField("url", url).Info("sending bundle creation request")

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

	if resp.StatusCode == http.StatusConflict {
		return nil, &DiagnosticsBundleAlreadyExists{id: ID}
	} else if resp.StatusCode != http.StatusOK {
		return nil, handleErrorCode(resp, url)
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

	logrus.WithField("ID", ID).WithField("url", url).Info("checking status of bundle")

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

	if resp.StatusCode == http.StatusNotFound {
		return nil, &DiagnosticsBundleNotFoundError{id: ID}
	} else if resp.StatusCode == http.StatusInternalServerError {
		return nil, &DiagnosticsBundleUnreadableError{id: ID}
	} else if resp.StatusCode != http.StatusOK {
		return nil, handleErrorCode(resp, url)
	}

	err = json.NewDecoder(resp.Body).Decode(bundle)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (d DiagnosticsClient) GetFile(ctx context.Context, node string, ID string, path string) error {
	url := fmt.Sprintf("%s/file", remoteURL(node, ID))

	logrus.WithField("ID", ID).WithField("url", url).Info("downloading local bundle from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &DiagnosticsBundleNotFoundError{id: ID}
	} else if resp.StatusCode == http.StatusInternalServerError {
		return &DiagnosticsBundleUnreadableError{id: ID}
	} else if resp.StatusCode != http.StatusOK {
		return handleErrorCode(resp, url)
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

	logrus.WithField("node", node).Info("getting list of bundles from node")

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, handleErrorCode(resp, url)
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

	logrus.WithField("node", node).WithField("ID", id).Info("deleting bundle from node")

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &DiagnosticsBundleNotCompletedError{id: id}
	} else if resp.StatusCode == http.StatusNotFound {
		return &DiagnosticsBundleNotFoundError{id: id}
	} else if resp.StatusCode == http.StatusInternalServerError {
		return &DiagnosticsBundleUnreadableError{id: id}
	} else if resp.StatusCode != http.StatusOK {
		return handleErrorCode(resp, url)
	}

	return nil
}

func handleErrorCode(resp *http.Response, url string) error {
	body := make([]byte, 100)
	resp.Body.Read(body)
	return fmt.Errorf("received unexpected status code [%d] from %s: %s", resp.StatusCode, url, string(body))
}

func remoteURL(node string, ID string) string {
	url := fmt.Sprintf("%s%s/%s", node, bundlesEndpoint, ID)
	return url
}

type DiagnosticsBundleNotFoundError struct {
	id string
}

func (d *DiagnosticsBundleNotFoundError) Error() string {
	return fmt.Sprintf("bundle %s not found", d.id)
}

type DiagnosticsBundleUnreadableError struct {
	id string
}

func (d *DiagnosticsBundleUnreadableError) Error() string {
	return fmt.Sprintf("bundle %s not readable", d.id)
}

type DiagnosticsBundleNotCompletedError struct {
	id string
}

func (d *DiagnosticsBundleNotCompletedError) Error() string {
	return fmt.Sprintf("bundle %s canceled or already deleted", d.id)
}

type DiagnosticsBundleAlreadyExists struct {
	id string
}

func (d *DiagnosticsBundleAlreadyExists) Error() string {
	return fmt.Sprintf("bundle %s already exists", d.id)
}
