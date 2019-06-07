package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

// Client is an interface that can talk with dcos-diagnostics REST API and manipulate remote bundles
type Client interface {
	// Create ask node ip to start bundle creation process with given id
	Create(ctx context.Context, node node, id string) (*Bundle, error)
	// Status returns status of bundle with given id on node with given ip
	Status(ctx context.Context, node node, id string) (*Bundle, error)
	// GetFile downloads bundle file of bundle with given id from node with given ip and save it to local filesystem.
	// It returns path where file is stored or error if there were a problem.
	GetFile(ctx context.Context, node node, id string) (path string, err error)
}

type Coordinator struct {
	client Client
}

type node struct {
	IP   net.IP `json:"ip"`
	Role string `json:"role"`
}

type bundleStatus struct {
	ID   string
	node node
	done bool
	err  error
}

type DiagnosticsClient struct {
	client        *http.Client
	forceTLS      bool
	getPortByRole func(role string) (int, error)
}

func newDiagnosticsClient(cfg *config.Config) *DiagnosticsClient {
	return &DiagnosticsClient{
		// TODO(br-lewis): this feels a little hacky
		getPortByRole: func(role string) (int, error) {
			switch role {
			case dcos.AgentRole:
				fallthrough
			case dcos.AgentPublicRole:
				return cfg.FlagAgentPort, nil
			case dcos.MasterRole:
				return cfg.FlagMasterPort, nil
			default:
				return 0, fmt.Errorf("incorrect role given %s", role)
			}
		},
	}
}

func (d *DiagnosticsClient) Create(ctx context.Context, node node, id string) (*Bundle, error) {
	logrus.WithField("bundle", id).WithField("node", node.IP).Info("sending bundle creation request")

	port, err := d.getPortByRole(node.Role)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s", node.IP, port, id)
	fullURL, err := util.UseTLSScheme(url, d.forceTLS)
	if err != nil {
		return nil, err
	}

	type payload struct {
		Type string `json:"type"`
	}

	body, err := json.Marshal(payload{
		Type: "LOCAL",
	})
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPut, fullURL, bytes.NewBuffer(body))
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

func (d *DiagnosticsClient) Status(ctx context.Context, node node, id string) (*Bundle, error) {
	logrus.WithField("bundle", id).WithField("node", node.IP).Info("checking status of bundle")

	port, err := d.getPortByRole(node.Role)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s", node.IP, port, id)
	fullURL, err := util.UseTLSScheme(url, d.forceTLS)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodGet, fullURL, nil)
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
		return nil, fmt.Errorf("received unexpected status code from %s: %d", node.IP, resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&bundle)
	if err != nil {
		return nil, err
	}

	return &bundle, nil
}

func (d *DiagnosticsClient) GetFile(ctx context.Context, node node, id string) (string, error) {
	logrus.Infof("downloading bundle %s from node %s", id, node.IP)

	port, err := d.getPortByRole(node.Role)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("http://%s:%d/system/health/v1/diagnostics/%s/file", node.IP, port, id)
	fullURL, err := util.UseTLSScheme(url, d.forceTLS)
	if err != nil {
		return "", nil
	}

	request, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := d.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("bundle %s not know to node %s", id, node.IP)
	} else if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received unexpected status %d", resp.StatusCode)
	}

	destinationName := fmt.Sprintf("%s-%s.zip", id, node.IP)
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
