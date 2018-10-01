package dcos

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/dcos/dcos-diagnostics/util"
)

// GetHostname return a localhost hostname.
func (st *Tools) GetHostname() (string, error) {
	if st.hostname != "" {
		return st.hostname, nil
	}
	var err error
	st.hostname, err = os.Hostname()
	if err != nil {
		return "", err
	}
	return st.hostname, nil
}

// DetectIP returns a detected IP by running /opt/mesosphere/bin/detect_ip. It will run only once and cache the result.
// When the function is called again, ip will be taken from cache.
func (st *Tools) DetectIP() (string, error) {
	ip, err := st.NodeInfo.DetectIP()
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

// GetNodeRole returns a nodes role. It will run only once and cache the result.
// When the function is called again, ip will be taken from cache.
func (st *Tools) GetNodeRole() (string, error) {
	if st.Role == "" {
		return "", errors.New("Could not determine a role, no /etc/mesosphere/roles/{master,slave,slave_public} file found")
	}
	return st.Role, nil
}

// GetMesosNodeID return a mesos node id.
func (st *Tools) GetMesosNodeID() (string, error) {
	// TODO(janisz): We need to decide if we need a context
	return st.NodeInfo.MesosID(context.TODO())
}

func (st *Tools) doRequest(method, url string, timeout time.Duration, body io.Reader) (responseBody []byte, httpResponseCode int, err error) {
	if url != st.ExhibitorURL {
		url, err = util.UseTLSScheme(url, st.ForceTLS)
		if err != nil {
			return responseBody, http.StatusBadRequest, err
		}
	}

	logrus.Debugf("[%s] %s, timeout: %s, forceTLS: %v, basicURL: %s", method, url, timeout.String(), st.ForceTLS, url)
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return responseBody, http.StatusBadRequest, err
	}

	client := util.NewHTTPClient(timeout, st.Transport)
	resp, err := client.Do(request)
	if err != nil {
		return responseBody, http.StatusBadRequest, err
	}

	defer resp.Body.Close()
	responseBody, err = ioutil.ReadAll(resp.Body)
	return responseBody, resp.StatusCode, nil
}

// Get HTTP request.
func (st *Tools) Get(url string, timeout time.Duration) (body []byte, httpResponseCode int, err error) {
	return st.doRequest("GET", url, timeout, nil)
}

// Post HTTP request.
func (st *Tools) Post(url string, timeout time.Duration) (body []byte, httpResponseCode int, err error) {
	return st.doRequest("POST", url, timeout, nil)
}

// GetTimestamp return time.Now()
func (st *Tools) GetTimestamp() time.Time {
	return time.Now()
}

// GetMasterNodes finds DC/OS masters.
func (st *Tools) GetMasterNodes() (nodesResponse []Node, err error) {
	finder := &findMastersInExhibitor{
		url:   st.ExhibitorURL,
		getFn: st.Get,
		next: &findNodesInDNS{
			forceTLS:  st.ForceTLS,
			dnsRecord: "master.mesos",
			role:      MasterRole,
			next:      nil,
		},
	}
	return finder.Find()
}

// GetAgentNodes finds DC/OS agents.
func (st *Tools) GetAgentNodes() (nodes []Node, err error) {
	finder := &findNodesInDNS{
		forceTLS:  st.ForceTLS,
		dnsRecord: "leader.mesos",
		role:      AgentRole,
		getFn:     st.Get,
		next: &FindAgentsInHistoryService{
			PastTime: "/minute/",
			next: &FindAgentsInHistoryService{
				PastTime: "/hour/",
				next:     nil,
			},
		},
	}
	return finder.Find()
}
