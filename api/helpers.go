package api

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	netUrl "net/url"
	"os"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"

	"github.com/sirupsen/logrus"
)

const (
	// _SYSTEMD_UNIT and UNIT are custom fields used by systemd to mark logs by the systemd unit itself and
	// also by other related components. When dcos-diagnostics reads log entries it needs to filter both entries.
	systemdUnitProperty = "_SYSTEMD_UNIT"
	unitProperty        = "UNIT"
)

// GetHostname return a localhost hostname.
func (st *DCOSTools) GetHostname() (string, error) {
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
func (st *DCOSTools) DetectIP() (string, error) {
	ip, err := st.NodeInfo.DetectIP()
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

// GetNodeRole returns a nodes role. It will run only once and cache the result.
// When the function is called again, ip will be taken from cache.
func (st *DCOSTools) GetNodeRole() (string, error) {
	if st.Role == "" {
		return "", errors.New("Could not determine a role, no /etc/mesosphere/roles/{master,slave,slave_public} file found")
	}
	return st.Role, nil
}

func useTLSScheme(url string, use bool) (string, error) {
	if use {
		urlObject, err := netUrl.Parse(url)
		if err != nil {
			return "", err
		}
		urlObject.Scheme = "https"
		return urlObject.String(), nil
	}
	return url, nil
}

// GetMesosNodeID return a mesos node id.
func (st *DCOSTools) GetMesosNodeID() (string, error) {
	// TODO(janisz): We need to decide if we need a context
	return st.NodeInfo.MesosID(context.TODO())
}

// Help functions
func isInList(item string, l []string) bool {
	for _, listItem := range l {
		if item == listItem {
			return true
		}
	}
	return false
}

func (st *DCOSTools) doRequest(method, url string, timeout time.Duration, body io.Reader) (responseBody []byte, httpResponseCode int, err error) {
	if url != st.ExhibitorURL {
		url, err = useTLSScheme(url, st.ForceTLS)
		if err != nil {
			return responseBody, http.StatusBadRequest, err
		}
	}

	logrus.Debugf("[%s] %s, timeout: %s, forceTLS: %v, basicURL: %s", method, url, timeout.String(), st.ForceTLS, url)
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return responseBody, http.StatusBadRequest, err
	}

	client := NewHTTPClient(timeout, st.Transport)
	resp, err := client.Do(request)
	if err != nil {
		return responseBody, http.StatusBadRequest, err
	}

	defer resp.Body.Close()
	responseBody, err = ioutil.ReadAll(resp.Body)
	return responseBody, resp.StatusCode, nil
}

// Get HTTP request.
func (st *DCOSTools) Get(url string, timeout time.Duration) (body []byte, httpResponseCode int, err error) {
	return st.doRequest("GET", url, timeout, nil)
}

// Post HTTP request.
func (st *DCOSTools) Post(url string, timeout time.Duration) (body []byte, httpResponseCode int, err error) {
	return st.doRequest("POST", url, timeout, nil)
}

// GetTimestamp return time.Now()
func (st *DCOSTools) GetTimestamp() time.Time {
	return time.Now()
}

// GetMasterNodes finds DC/OS masters.
func (st *DCOSTools) GetMasterNodes() (nodesResponse []dcos.Node, err error) {
	finder := &findMastersInExhibitor{
		url:   st.ExhibitorURL,
		getFn: st.Get,
		next: &findNodesInDNS{
			forceTLS:  st.ForceTLS,
			dnsRecord: "master.mesos",
			role:      dcos.MasterRole,
			next:      nil,
		},
	}
	return finder.find()
}

// GetAgentNodes finds DC/OS agents.
func (st *DCOSTools) GetAgentNodes() (nodes []dcos.Node, err error) {
	finder := &findNodesInDNS{
		forceTLS:  st.ForceTLS,
		dnsRecord: "leader.mesos",
		role:      dcos.AgentRole,
		getFn:     st.Get,
		next: &findAgentsInHistoryService{
			pastTime: "/minute/",
			next: &findAgentsInHistoryService{
				pastTime: "/hour/",
				next:     nil,
			},
		},
	}
	return finder.find()
}

// NewHTTPClient creates a new instance of http.Client
func NewHTTPClient(timeout time.Duration, transport http.RoundTripper) *http.Client {
	client := &http.Client{
		Timeout: timeout,
	}

	if transport != nil {
		client.Transport = transport
	}

	// go http client does not copy the headers when it follows the redirect.
	// https://github.com/golang/go/issues/4800
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		for attr, val := range via[0].Header {
			if _, ok := req.Header[attr]; !ok {
				req.Header[attr] = val
			}
		}
		return nil
	}

	return client
}

// open a file for reading, a caller if responsible to close a file descriptor.
func readFile(fileLocation string) (r io.ReadCloser, err error) {
	file, err := os.Open(fileLocation)
	if err != nil {
		return r, err
	}
	return file, nil
}
