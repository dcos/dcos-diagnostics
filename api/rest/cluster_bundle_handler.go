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
	coord      coordinator
	client     Client
	tools      dcos.Tooler
	timeout    time.Duration
	clock      Clock
	urlBuilder dcos.NodeURLBuilder
}

func NewClusterBundleHandler(c coordinator, client Client, tools dcos.Tooler, workDir string, timeout time.Duration,
	clock Clock, urlBuilder dcos.NodeURLBuilder) *ClusterBundleHandler {

	return &ClusterBundleHandler{
		coord:      c,
		client:     client,
		workDir:    workDir,
		timeout:    timeout,
		tools:      tools,
		clock:      clock,
		urlBuilder: urlBuilder,
	}
}

// Create will send the initial creation request for the bundle to all nodes. The created
// bundle will exist on the called master node
func (c *ClusterBundleHandler) Create(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if c.bundleExists(id) {
		writeJSONError(w, http.StatusConflict, fmt.Errorf("bundle %s already exists", id))
		return
	}

	bundleWorkDir := filepath.Join(c.workDir, id)
	err := os.MkdirAll(bundleWorkDir, dirPerm)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create bundle %s workdir: %s", id, err))
		return
	}

	stateFilePath := filepath.Join(c.workDir, id, stateFileName)

	bundle := Bundle{
		ID:      id,
		Started: c.clock.Now(),
		Status:  Started,
	}

	bundleStatus := jsonMarshal(bundle)
	err = ioutil.WriteFile(stateFilePath, bundleStatus, filePerm)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not update state file %s: %s", id, err))
		return
	}

	dataFile, err := os.Create(filepath.Join(c.workDir, id, dataFileName))
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create data file %s: %s", id, err))
		return
	}
	masters, err := c.tools.GetMasterNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error getting master nodes for bundle %s: %s", id, err))
		return
	}
	agents, err := c.tools.GetAgentNodes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("error getting agent nodes for bundle %s: %s", id, err))
		return
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
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to create local bundle id for bundle %s: %s", id, err))
		return
	}
	statuses := c.coord.CreateBundle(ctx, localBundleID.String(), nodes)

	go c.waitAndCollectRemoteBundle(ctx, &bundle, len(nodes), dataFile, stateFilePath, statuses)

	write(w, bundleStatus)
}

func (c *ClusterBundleHandler) waitAndCollectRemoteBundle(ctx context.Context, bundle *Bundle, numBundles int,
	dataFile io.WriteCloser, stateFilePath string, statuses <-chan bundleStatus) {

	defer dataFile.Close()

	bundleFilePath, err := c.coord.CollectBundle(ctx, bundle.ID, numBundles, statuses)
	if err != nil {
		bundle.Errors = append(bundle.Errors, err.Error())
	}
	bundle.Stopped = c.clock.Now()
	bundle.Status = Done

	err = ioutil.WriteFile(stateFilePath, jsonMarshal(bundle), filePerm)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("Could not update state file.")
		return
	}

	bundleFile, err := os.Open(bundleFilePath)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("unable to open bundle for copying")
		return
	}

	_, err = io.Copy(dataFile, bundleFile)
	if err != nil {
		logrus.WithError(err).WithField("ID", bundle.ID).Error("unable to copy bundle from temp dir working directory")
		return
	}
}

// List will get a list of all bundles available across all masters
func (c *ClusterBundleHandler) List(w http.ResponseWriter, r *http.Request) {
	masters, err := c.getClusterNodes()
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

	ctx := context.TODO()

	// TODO: parallelize this
	// TODO: it's very possible that we can have duplicate node IDs for the local bundles that will be generated on the master
	for _, n := range masters {
		bundle, err := c.client.Status(ctx, n.baseURL, id)

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
			switch err.(type) {
			case *DiagnosticsBundleNotCompletedError:
				writeJSONError(w, http.StatusNotModified, err)
				return
			case *DiagnosticsBundleUnreadableError:
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
	for _, n := range masters {
		_, err = c.client.Status(ctx, n.baseURL, id)
		if err != nil {
			masterWithBundle = n
			break
		}
	}

	bundleDir, err := ioutil.TempDir("", "bundle-*")
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

	data, err := ioutil.ReadFile(bundleFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("unable to read downloaded bundle file: %s", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (c *ClusterBundleHandler) getClusterNodes() ([]node, error) {
	masters, err := c.getMasterNodes()
	if err != nil {
		return nil, err
	}

	agents, err := c.getAgentNodes()
	if err != nil {
		return nil, err
	}

	return append(masters, agents...), nil
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

func (c *ClusterBundleHandler) getAgentNodes() ([]node, error) {
	agents, err := c.tools.GetAgentNodes()
	if err != nil {
		return nil, err
	}

	nodes := []node{}
	for _, n := range agents {
		ip := net.ParseIP(n.IP)
		// govet seems to have an issue with err shadowing a previous declaration, not sure why
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

func (c *ClusterBundleHandler) getBundleState(id string) (Bundle, error) {
	bundle := Bundle{
		ID:     id,
		Status: Unknown,
	}

	stateFilePath := filepath.Join(c.workDir, id, stateFileName)
	rawState, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return bundle, fmt.Errorf("could not read state file for bundle %s: %s", id, err)
	}

	err = json.Unmarshal(rawState, &bundle)
	if err != nil {
		return bundle, fmt.Errorf("could not unmarshal state file %s: %s", id, err)
	}

	if bundle.Status == Deleted || bundle.Status == Canceled || bundle.Status == Unknown {
		return bundle, nil
	}

	dataFileStat, err := os.Stat(filepath.Join(c.workDir, id, dataFileName))
	if err != nil {
		bundle.Status = Unknown
		return bundle, fmt.Errorf("could not stat data file %s: %s", id, err)
	}

	if bundle.Size != dataFileStat.Size() {
		bundle.Size = dataFileStat.Size()
		// Update status files
		bundleStatus := jsonMarshal(bundle)
		err = ioutil.WriteFile(stateFilePath, bundleStatus, filePerm)
		if err != nil {
			return bundle, fmt.Errorf("could not update state file %s: %s", id, err)
		}
	}

	return bundle, nil
}
