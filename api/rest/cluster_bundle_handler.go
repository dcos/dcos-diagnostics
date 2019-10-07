package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// ClusterBundleHandler is a handler that will create and manage cluster-wide
// diagnostics bundles
type ClusterBundleHandler struct {
	workDir    string
	coord      Coordinator
	client     Client
	tools      dcos.Tooler
	timeout    time.Duration
	clock      Clock
	urlBuilder dcos.NodeURLBuilder
}

func NewClusterBundleHandler(c Coordinator, client Client, tools dcos.Tooler, workDir string, timeout time.Duration,
	urlBuilder dcos.NodeURLBuilder) (*ClusterBundleHandler, error) {
	err := initializeWorkDir(workDir)
	if err != nil {
		return nil, err
	}

	return &ClusterBundleHandler{
		coord:      c,
		client:     client,
		workDir:    workDir,
		timeout:    timeout,
		tools:      tools,
		clock:      &realClock{},
		urlBuilder: urlBuilder,
	}, nil
}

// Create will send the initial creation request for the bundle to all nodes. The created
// bundle will exist on the called master node
func (c *ClusterBundleHandler) Create(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	options, err := getOptionsFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("could not parse request body %s", err))
		return
	}

	if c.bundleExists(id) {
		writeJSONError(w, http.StatusConflict, fmt.Errorf("bundle %s already exists", id))
		return
	}

	bundleWorkDir := filepath.Join(c.workDir, id)
	err = os.MkdirAll(bundleWorkDir, dirPerm)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create bundle %s workdir: %s", id, err))
		return
	}

	bundle := Bundle{
		ID:      id,
		Type:    Cluster,
		Started: c.clock.Now(),
		Status:  Started,
	}

	bundleStatus, err := c.writeStateFile(bundle)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, err)
		return
	}

	dataFile, err := os.Create(filepath.Join(c.workDir, id, dataFileName))
	if err != nil {
		if e := c.failed(bundle, err); e != nil {
			logrus.WithField("ID", bundle.ID).Error(e.Error())
		}
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create data file %s: %s", id, err))
		return
	}

	var masters, agents []dcos.Node

	if options.Masters {
		masters, err = c.tools.GetMasterNodes()
		if err != nil {
			if e := c.failed(bundle, err); e != nil {
				logrus.WithField("ID", bundle.ID).Error(e.Error())
			}
			writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error getting master nodes for bundle %s: %s", id, err))
			return
		}
	}

	if options.Agents == true {
		agents, err = c.tools.GetAgentNodes()
		if err != nil {
			if e := c.failed(bundle, err); e != nil {
				logrus.WithField("ID", bundle.ID).Error(e.Error())
			}
			writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error getting agent nodes for bundle %s: %s", id, err))
			return
		}
	}

	allNodes := append(masters, agents...)
	nodes := make([]node, 0, len(allNodes))
	for _, n := range allNodes {
		ip := net.ParseIP(n.IP)
		// govet seems to have an issue with err shadowing a previous declaration, not sure why
		//nolint:govet
		url, err := c.urlBuilder.BaseURL(ip, n.Role)
		if err != nil {
			logrus.WithField("bundle", id).WithField("node", ip).WithField("role", n.Role).WithError(err).Error("unable to build base URL for node, skipping")
			continue
		}
		nodes = append(nodes, node{
			Role:    n.Role,
			IP:      ip,
			baseURL: url,
		})
	}

	//TODO(janisz): use context cancel function to cancel bundle creation https://jira.mesosphere.com/browse/DCOS_OSS-5222
	//nolint:govet
	ctx, _ := context.WithTimeout(context.Background(), c.timeout)

	localBundleID, err := uuid.NewUUID()
	if err != nil {
		if e := c.failed(bundle, err); e != nil {
			logrus.WithField("ID", bundle.ID).Error(e.Error())
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to create local bundle id for bundle %s: %s", id, err))
		return
	}
	statuses := c.coord.CreateBundle(ctx, localBundleID.String(), nodes)

	go c.waitAndCollectRemoteBundle(ctx, bundle, len(nodes), dataFile, statuses)

	write(w, bundleStatus)
}

type options struct {
	Masters bool `json:"masters"`
	Agents  bool `json:"agents"`
}

var defaultOptions = options{
	Masters: true,
	Agents:  true,
}

func getOptionsFromRequest(r *http.Request) (options, error) {
	o := defaultOptions
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
			if err != io.EOF { // Accept empty body
				return o, err
			}
		}
	}
	return o, nil
}

func (c *ClusterBundleHandler) failed(bundle Bundle, err error) error {
	bundle.Failed(c.clock.Now(), err)
	_, e := c.writeStateFile(bundle)
	return e
}

func (c *ClusterBundleHandler) waitAndCollectRemoteBundle(ctx context.Context, bundle Bundle, numBundles int,
	dataFile io.WriteCloser, statuses <-chan BundleStatus) {

	defer dataFile.Close()

	bundleFilePath, err := c.coord.CollectBundle(ctx, bundle.ID, numBundles, statuses)
	if err != nil {
		bundle.Errors = append(bundle.Errors, err.Error())
	}

	bundleFile, err := os.Open(bundleFilePath)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("unable to open bundle for copying")
		if e := c.failed(bundle, err); e != nil {
			logrus.WithField("ID", bundle.ID).Error(e.Error())
		}
		return
	}

	_, err = io.Copy(dataFile, bundleFile)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("unable to copy bundle from temp dir working directory")
		if e := c.failed(bundle, err); e != nil {
			logrus.WithField("ID", bundle.ID).Error(e.Error())
		}
		return
	}

	bundle.Stopped = c.clock.Now()
	bundle.Status = Done

	_, err = c.writeStateFile(bundle)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("Could not update state file.")
		return
	}
}

func (c *ClusterBundleHandler) writeStateFile(bundle Bundle) ([]byte, error) {
	stateFilePath := filepath.Join(c.workDir, bundle.ID, stateFileName)
	bundleStatus := jsonMarshal(bundle)
	err := ioutil.WriteFile(stateFilePath, bundleStatus, filePerm)
	if err != nil {
		err = fmt.Errorf("could not update state file %s: %s", bundle.ID, err)
	}
	return bundleStatus, err
}

// List will get a list of all bundles available across all masters
func (c *ClusterBundleHandler) List(w http.ResponseWriter, r *http.Request) {
	masters, err := c.getMasterNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to get list of master nodes: %s", err))
		return
	}

	ctx := context.Background()

	bundles := []*Bundle{}
	for _, n := range masters {
		nodeBundles, err := c.client.List(ctx, n.baseURL)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to get list of bundles from all masters: %s", err))
			return
		}
		bundles = append(bundles, nodeBundles...)
	}

	write(w, jsonMarshal(bundles))
}

// Status will return the status of a given bundle, proxying the call to the appropriate master
func (c *ClusterBundleHandler) Status(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	masters, err := c.getMasterNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to get list of master nodes: %s", err))
		return
	}

	ctx := context.Background()

	// TODO: parallelize this
	// TODO: it's very possible that we can have duplicate node IDs for the local bundles that will be generated on the master
	for _, n := range masters {
		bundle, err := c.client.Status(ctx, n.baseURL, id)
		if err != nil {
			if _, ok := err.(*DiagnosticsBundleUnreadableError); ok {
				writeJSONError(w, http.StatusInternalServerError, err)
				return
			}
		}

		if err == nil {
			write(w, jsonMarshal(bundle))
			return
		}
	}

	// we would only get here if we didn't find the bundle on any of the masters
	writeJSONError(w, http.StatusNotFound, fmt.Errorf("bundle %s did not exist on any masters", id))
}

// Delete will delete a given bundle, proxying the call if the given bundle exists
// on a different master
func (c *ClusterBundleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	masters, err := c.getMasterNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to get list of masters: %s", err))
		return
	}

	ctx := context.Background()

	found := false
	for _, n := range masters {
		err = c.client.Delete(ctx, n.baseURL, id)
		if err != nil {
			// some errors tell us the bundle was found on a master but something else was wrong so we end and return an error status
			// but a NotFound error means it should keep going
			if _, ok := err.(*DiagnosticsBundleUnreadableError); ok {
				writeJSONError(w, http.StatusInternalServerError, err)
				return
			}
		}
		if err == nil {
			found = true
		}
	}

	if !found {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("bundle %s not found on any master", id))
	}
}

// Download will download the given bundle, proxying the call to the appropriate master
func (c *ClusterBundleHandler) Download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// TODO: for this one specifically, it would be ideal to detect if the bundle exists on the calling master
	// first since then we can skip the intermediate download
	masters, err := c.getMasterNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to get list of masters: %s", err))
		return
	}

	ctx := context.Background()

	var masterWithBundle node
	found := false
	for _, n := range masters {
		bundle, statusErr := c.client.Status(ctx, n.baseURL, id)
		if statusErr != nil {
			switch statusErr.(type) {
			case *DiagnosticsBundleUnreadableError:
				writeJSONError(w, http.StatusInternalServerError, statusErr)
				return
			case *DiagnosticsBundleNotFoundError:
				continue
			}
		}

		if bundle.Status == Done {
			masterWithBundle = n
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, &DiagnosticsBundleNotFoundError{id: id})
		return
	}

	bundleDir, err := ioutil.TempDir("", "bundle-")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error opening temp file to download bundle %s", err))
		return
	}
	defer os.RemoveAll(bundleDir)

	bundleFilename := filepath.Join(bundleDir, "bundle.zip")

	err = c.client.GetFile(ctx, masterWithBundle.baseURL, id, bundleFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error downloading bundle: %s", err))
		return
	}
	w.Header().Add("Content-Type", "application/zip, application/octet-stream")
	w.Header().Add("Content-disposition", fmt.Sprintf("attachment; filename=%s.zip", id))
	http.ServeFile(w, r, bundleFilename)
}

func (c *ClusterBundleHandler) getMasterNodes() ([]node, error) {
	masters, err := c.tools.GetMasterNodes()
	if err != nil {
		return nil, err
	}
	nodes := []node{}
	for _, n := range masters {
		ip := net.ParseIP(n.IP)
		url, err := c.urlBuilder.BaseURL(ip, n.Role)
		if err != nil {
			logrus.WithField("node", ip).WithField("role", n.Role).WithError(err).Error("unable to build base URL for node, skipping")
			continue
		}
		nodes = append(nodes, node{
			Role:    n.Role,
			IP:      ip,
			baseURL: url,
		})
	}

	return nodes, nil
}

func (c *ClusterBundleHandler) bundleExists(id string) bool {
	s, err := os.Stat(filepath.Join(c.workDir, id))
	if os.IsNotExist(err) {
		return false
	}
	if !s.IsDir() {
		// If this is a file then it's not a valid bundle
		return false
	}
	return true
}
