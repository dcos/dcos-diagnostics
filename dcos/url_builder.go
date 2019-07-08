package dcos

import (
	"fmt"
	"net"

	"github.com/dcos/dcos-diagnostics/util"
)

// NodeURLBuilder is an interface to define how to map from a node's IP and role
// to a URL to reach the node's HTTP API
type NodeURLBuilder interface {
	BaseURL(ip net.IP, role string) (string, error)
}

// URLBuilder implements NodeURLBuilder mapping agent and master roles to their
// configured ports and handling if the cluster requires TLS
type URLBuilder struct {
	agentPort  int
	masterPort int
	forceTLS   bool
}

// NewURLBuilder constructs a NodeURLBuilder
func NewURLBuilder(agentPort int, masterPort int, forceTLS bool) URLBuilder {
	return URLBuilder{
		agentPort:  agentPort,
		masterPort: masterPort,
		forceTLS:   forceTLS,
	}
}

// BaseURL will return the base URL for a node given its role and whether the
// cluster is configured to require TLS.
func (n *URLBuilder) BaseURL(ip net.IP, role string) (string, error) {
	port, err := n.port(role)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("http://%s:%d", ip, port)
	fullURL, err := util.UseTLSScheme(url, n.forceTLS)
	if err != nil {
		return "", err
	}

	return fullURL, nil
}

func (n *URLBuilder) port(role string) (int, error) {
	switch role {
	case AgentRole:
		fallthrough
	case AgentPublicRole:
		return n.agentPort, nil
	case MasterRole:
		return n.masterPort, nil
	default:
		return 0, fmt.Errorf("incorrect role given %s", role)
	}
}
