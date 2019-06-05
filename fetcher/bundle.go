package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dcos/dcos-diagnostics/api/rest"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

// BundleRequest represents a request for a node's diagnostic bundle
type BundleRequest struct {
	BundleID string
	Node     *dcos.Node
	Client   *http.Client
	port     int
	forceTLS bool
	Data     []byte
}

// RequestStatus is a periodic status update about the progress of a bundle creation request
type RequestStatus struct {
	Request  *BundleRequest
	Finished bool
	Err      error
}

// NewBundleRequest creates a new bundle request object
func NewBundleRequest(id string, node dcos.Node, client *http.Client, port int, forceTLS bool) *BundleRequest {
	return &BundleRequest{
		BundleID: id,
		Node:     &node,
		Client:   client,
		port:     port,
		forceTLS: forceTLS,
		Data:     nil,
	}
}

// SendCreationRequest will send a bundle creation request to the given node
func (b *BundleRequest) SendCreationRequest(ctx context.Context, requestFinished chan<- RequestStatus) error {
	logrus.Infof("Sending bundle creation request to %s", b.Node.IP)

	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s", b.Node.IP, b.port, b.BundleID)
	fullURL, err := util.UseTLSScheme(url, b.forceTLS)

	if err != nil {
		return err
	}

	type payload struct {
		Type string `json:"type"`
	}

	body, err := json.Marshal(payload{
		Type: "LOCAL",
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequest("PUT", fullURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	request.WithContext(ctx)

	resp, err := b.Client.Do(request)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status code received: %d", resp.StatusCode)
	}

	// start update checker
	go statusUpdateCheck(ctx, b, requestFinished)

	return nil
}

func (b *BundleRequest) checkStatus(ctx context.Context) RequestStatus {
	status := RequestStatus{
		Request: b,
	}

	logrus.Infof("Checking status of bundle creation on node %s", b.Node.IP)
	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s", b.Node.IP, b.port, b.BundleID)

	fullURL, err := util.UseTLSScheme(url, b.forceTLS)
	if err != nil {
		// TODO
	}

	request, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		status.Err = err
		return status
	}

	request.WithContext(ctx)

	resp, err := b.Client.Do(request)
	if err != nil {
		status.Err = err
		return status
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var body rest.Bundle

		err = json.NewDecoder(resp.Body).Decode(&body)
		if err != nil {
			status.Err = fmt.Errorf("Error decoding status response: %s", err)
		}

		if body.Status == rest.Done || body.Status == rest.Canceled {
			status.Finished = true
		} else {
			status.Finished = false
		}
	} else if resp.StatusCode == 404 {
		status.Err = fmt.Errorf("Bundle ID %s not known to agent", b.BundleID)
		status.Finished = true
	} else {
		status.Err = fmt.Errorf("received unexpected status code %d", resp.StatusCode)
		status.Finished = true
	}

	return status
}

// Download will download the bundle file created by this Bundle Request,
// successful download will result in the data being put into b.Data
func (b *BundleRequest) Download(ctx context.Context) error {
	logrus.Infof("downloading bundle %s from node %s", b.BundleID, b.Node.IP)
	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s/file", b.Node.IP, b.port, b.BundleID)

	fullURL, err := util.UseTLSScheme(url, b.forceTLS)
	if err != nil {
		return err
	}

	request, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return err
	}

	request.WithContext(ctx)

	resp, err := b.Client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		b.Data = body
		return nil

	} else if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("bundle %s not found", b.BundleID)
	} else {
		return fmt.Errorf("unexpected status code while downloading bundle %s from node %s: %d", b.BundleID, b.Node.IP, resp.StatusCode)
	}
}

func statusUpdateCheck(ctx context.Context, b *BundleRequest, statusListener chan<- RequestStatus) {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		logrus.Infof("polling %s for bundle update", b.Node.IP)
		select {
		case <-ctx.Done():
			logrus.Infof("request context finished, marking bundle as canceled")
			ticker.Stop()
			statusListener <- RequestStatus{
				Request:  b,
				Finished: true,
				Err:      fmt.Errorf("Bundle canceled"),
			}
		default:
			status := b.checkStatus(ctx)
			if status.Finished {
				logrus.Infof("%s bundle finished", b.Node.IP)
				ticker.Stop()

				// only send an update when finished
				statusListener <- status
			} else {
				logrus.Infof("%s still working", b.Node.IP)
			}
		}
	}
}
