package dcos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/dcos/dcos-diagnostics/util"
)

// nodeFinder interface allows chain finding methods
type nodeFinder interface {
	Find() ([]Node, error)
}

var dcosHistoryPath = "/var/lib/dcos/dcos-history"

func (f *FindAgentsInHistoryService) getMesosAgents() (nodes []Node, err error) {
	basePath := dcosHistoryPath + f.PastTime
	files, err := ioutil.ReadDir(basePath)
	if err != nil {
		return nodes, err
	}
	nodeCount := make(map[string]int)
	for _, historyFile := range files {
		filePath := filepath.Join(basePath, historyFile.Name())
		agents, err := ioutil.ReadFile(filePath)
		if err != nil {
			logrus.Errorf("Could not read %s: %s", filePath, err)
			continue
		}

		unquotedAgents, err := strconv.Unquote(string(agents))
		if err != nil {
			logrus.Errorf("Could not unquote agents string %s: %s", string(agents), err)
			continue
		}

		var sr agentsResponse
		if err := json.Unmarshal([]byte(unquotedAgents), &sr); err != nil {
			logrus.Errorf("Could not unmarshal unquotedAgents %s: %s", unquotedAgents, err)
			continue
		}

		for _, agent := range sr.Agents {
			if _, ok := nodeCount[agent.Hostname]; ok {
				nodeCount[agent.Hostname]++
			} else {
				nodeCount[agent.Hostname] = 1
			}
		}

	}
	if len(nodeCount) == 0 {
		return nodes, NodesNotFoundError{
			msg: fmt.Sprintf("Agent nodes were not found in history service for the past %s", f.PastTime),
		}
	}

	for ip := range nodeCount {
		nodes = append(nodes, Node{
			Role: AgentRole,
			IP:   ip,
		})
	}
	return nodes, nil
}

// Find returns list of nodes or error if nodes could not be detected
func (f *FindAgentsInHistoryService) Find() (nodes []Node, err error) {
	nodes, err = f.getMesosAgents()
	if err == nil {
		logrus.Debugf("Found agents in the history service for past %s", f.PastTime)
		return nodes, nil
	}
	// try next provider if it is available
	if f.next != nil {
		logrus.Warning(err)
		return f.next.Find()
	}
	return nodes, err
}

// Find masters via dns. Used to Find master nodes from agents.
type findMastersInExhibitor struct {
	url  string
	next nodeFinder

	// getFn takes url and timeout and returns a read body, HTTP status code and error.
	getFn func(string, time.Duration) ([]byte, int, error)
}

type exhibitorNodeResponse struct {
	Code        int
	Description string
	Hostname    string
	IsLeader    bool
}

func (f *findMastersInExhibitor) findMesosMasters() (nodes []Node, err error) {
	if f.getFn == nil {
		return nodes, errors.New("could not initialize HTTP GET function. Make sure you set getFn in the constructor")
	}
	timeout := time.Second * 11
	body, statusCode, err := f.getFn(f.url, timeout)
	if err != nil {
		return nodes, err
	}
	if statusCode != http.StatusOK {
		return nodes, fmt.Errorf("GET %s failed, status code: %d", f.url, statusCode)
	}

	var exhibitorNodesResponse []exhibitorNodeResponse
	if err := json.Unmarshal(body, &exhibitorNodesResponse); err != nil {
		return nodes, err
	}
	if len(exhibitorNodesResponse) == 0 {
		return nodes, errors.New("master nodes not found in exhibitor")
	}

	for _, exhibitorNodeResponse := range exhibitorNodesResponse {
		nodes = append(nodes, Node{
			Role:   MasterRole,
			IP:     exhibitorNodeResponse.Hostname,
			Leader: exhibitorNodeResponse.IsLeader,
		})
	}
	return nodes, nil
}

func (f *findMastersInExhibitor) Find() (nodes []Node, err error) {
	nodes, err = f.findMesosMasters()
	if err == nil {
		logrus.Debug("Found masters in exhibitor")
		return nodes, nil
	}
	// try next provider if it is available
	if f.next != nil {
		logrus.Warning(err)
		return f.next.Find()
	}
	return nodes, err
}

// NodesNotFoundError is a custom error called when nodes are not found.
type NodesNotFoundError struct {
	msg string
}

func (n NodesNotFoundError) Error() string {
	return n.msg
}

// FindAgentsInHistoryService returns agents from dcos-history service files
type FindAgentsInHistoryService struct {
	PastTime string
	next     nodeFinder
}

// Find agents by resolving dns entry
type findNodesInDNS struct {
	forceTLS  bool
	dnsRecord string
	role      string
	next      nodeFinder

	// getFn takes url and timeout and returns a read body, HTTP status code and error.
	getFn func(string, time.Duration) ([]byte, int, error)
}

// Agent response json format
type agentsResponse struct {
	Agents []struct {
		Hostname   string `json:"hostname"`
		Attributes struct {
			PublicIP string `json:"public_ip"`
		} `json:"attributes"`
	} `json:"slaves"`
}

func (f *findNodesInDNS) resolveDomain() (ips []string, err error) {
	return net.LookupHost(f.dnsRecord)
}

func (f *findNodesInDNS) getMesosMasters() (nodes []Node, err error) {
	ips, err := f.resolveDomain()
	if err != nil {
		return nodes, err
	}
	if len(ips) == 0 {
		return nodes, errors.New("Could not resolve " + f.dnsRecord)
	}

	for _, ip := range ips {
		nodes = append(nodes, Node{
			Role: MasterRole,
			IP:   ip,
		})
	}
	return nodes, nil
}

func (f *findNodesInDNS) getMesosAgents() (nodes []Node, err error) {
	if f.getFn == nil {
		return nodes, errors.New("Could not initialize HTTP GET function. Make sure you set getFn in constractor")
	}
	leaderIps, err := f.resolveDomain()
	if err != nil {
		return nodes, err
	}
	if len(leaderIps) == 0 {
		return nodes, errors.New("Could not resolve " + f.dnsRecord)
	}

	url, err := util.UseTLSScheme(fmt.Sprintf("http://%s:5050/slaves", leaderIps[0]), f.forceTLS)
	if err != nil {
		return nodes, err
	}

	timeout := time.Second
	body, statusCode, err := f.getFn(url, timeout)
	if err != nil {
		return nodes, err
	}
	if statusCode != http.StatusOK {
		return nodes, fmt.Errorf("GET %s failed, status code %d", url, statusCode)
	}

	var sr agentsResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nodes, err
	}

	for _, agent := range sr.Agents {
		role := AgentRole

		// if a node has "attributes": {"public_ip": "true"} we consider it to be a public agent
		if agent.Attributes.PublicIP == "true" {
			role = AgentPublicRole
		}
		nodes = append(nodes, Node{
			Role: role,
			IP:   agent.Hostname,
		})
	}
	return nodes, nil
}

func (f *findNodesInDNS) dispatchGetNodesByRole() (nodes []Node, err error) {
	if f.role == MasterRole {
		return f.getMesosMasters()
	}
	if f.role != AgentRole {
		return nodes, fmt.Errorf("%s role is incorrect, must be %s or %s", f.role, MasterRole, AgentRole)
	}
	return f.getMesosAgents()
}

func (f *findNodesInDNS) Find() (nodes []Node, err error) {
	nodes, err = f.dispatchGetNodesByRole()
	if err == nil {
		logrus.Debugf("Found %s nodes by resolving %s", f.role, f.dnsRecord)
		return nodes, err
	}
	if f.next != nil {
		logrus.Warning(err)
		return f.next.Find()
	}
	return nodes, err
}
