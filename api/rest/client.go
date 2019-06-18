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

const bundlesEndpoint = "/system/health/v1/diagnostics"

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

	if resp.StatusCode != http.StatusOK {
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
		bundle.Status = Unknown
		return bundle, nil
	}
	if resp.StatusCode != http.StatusOK {
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

	if resp.StatusCode != http.StatusOK {
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

	// return the full path to the created file
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
