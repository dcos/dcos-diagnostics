package rest

import (
	"net/http"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"
)

// ClusterBundleHandler is a handler that will create and manage cluster-wide
// diagnostics bundles
type ClusterBundleHandler struct {
	workDir string
	coord   coordinator
	tools   dcos.Tooler
	timeout time.Duration
	clock   Clock
}

func NewClusterBundleHandler(c coordinator, tools dcos.Tooler, workDir string, timeout time.Duration, clock Clock) *ClusterBundleHandler {
	return &ClusterBundleHandler{
		coord:   c,
		workDir: workDir,
		timeout: timeout,
		tools:   tools,
		clock:   clock,
	}
}

// Create will send the initial creation request for the bundle to all nodes. The created
// bundle will exist on the called master node
func (c *ClusterBundleHandler) Create(w http.ResponseWriter, r *http.Request) {
	masters, err := c.tools.GetMasterNodes()
	if err != nil {
	}
	agents, err := c.tools.GetAgentNodes()
	if err != nil {
	}

	_ = append(masters, agents...)

}

// List will get a list of all bundles available across all masters
func (c *ClusterBundleHandler) List(w http.ResponseWriter, r *http.Request) {

}

// Status will return the status of a given bundle, proxying the call to the appropriate master
func (c *ClusterBundleHandler) Status(w http.ResponseWriter, r *http.Request) {

}

// Delete will delete a given bundle, proxying the call if the given bundle exists
// on a different master
func (c *ClusterBundleHandler) Delete(w http.ResponseWriter, r *http.Request) {

}

// Download will download the given bundle, proxying the call to the appropriate master
func (c *ClusterBundleHandler) Download(w http.ResponseWriter, r *http.Request) {

}
