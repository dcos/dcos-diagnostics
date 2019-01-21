package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type handler struct {
	cfg                *config.Config
	tools              dcos.Tooler
	job                *DiagnosticsJob
	systemdUnits       *SystemdUnits
	monitoringResponse *MonitoringResponse
}

// Route handlers
// /api/v1/system/health, get a units status, used by dcos-diagnostics puller
func (h *handler) unitsHealthStatus(w http.ResponseWriter, _ *http.Request) {
	health, err := h.systemdUnits.GetUnitsProperties(h.tools)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/units, get an array of all units collected from all hosts in a cluster
func (h *handler) getAllUnitsHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(h.monitoringResponse.GetAllUnits()); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/units/:unit_id:
func (h *handler) getUnitByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unitResponse, err := h.monitoringResponse.GetUnit(vars["unitid"])
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(unitResponse); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/units/:unit_id:/nodes
func (h *handler) getNodesByUnitIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodesForUnitResponse, err := h.monitoringResponse.GetNodesForUnit(vars["unitid"])
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(nodesForUnitResponse); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/units/:unit_id:/nodes/:node_id:
func (h *handler) getNodeByUnitIDNodeIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodePerUnit, err := h.monitoringResponse.GetSpecificNodeForUnit(vars["unitid"], vars["nodeid"])

	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(nodePerUnit); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// list the entire tree
func (h *handler) reportHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(h.monitoringResponse); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/nodes
func (h *handler) getNodesHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(h.monitoringResponse.GetNodes()); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/nodes/:node_id:
func (h *handler) getNodeByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodes, err := h.monitoringResponse.GetNodeByID(vars["nodeid"])
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// /api/v1/system/health/nodes/:node_id:/units
func (h *handler) getNodeUnitsByNodeIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	units, err := h.monitoringResponse.GetNodeUnitsID(vars["nodeid"])
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(units); err != nil {
		log.WithError(err).Error("Failed to encode responses to JSON")
		httpError(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *handler) getNodeUnitByNodeIDUnitIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	unit, err := h.monitoringResponse.GetNodeUnitByNodeIDUnitID(vars["nodeid"], vars["unitid"])
	if err != nil {
		if _, ok := err.(notFoundError); ok {
			httpError(w, err.Error(), http.StatusNotFound)
		} else {
			httpError(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(unit); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// diagnostics handlers
// A handler responsible for removing diagnostics bundles. First it will try to find a bundle locally, if failed
// it will send a broadcast request to all cluster master members and check if bundle it available.
// If a bundle was found on a remote host the local node will send a POST request to remove the bundle.
func (h *handler) deleteBundleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	response, err := h.job.delete(vars["file"])
	if err != nil {
		log.Errorf("Could not delete a file %s: %s", vars["file"], err)
	}
	writeResponse(w, response)
}

// A handler function return a diagnostics job status
func (h *handler) diagnosticsJobStatusHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(h.job.getBundleReportStatus()); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// A handler function returns a map of master node ip address as a key and bundleReportStatus as a value.
func (h *handler) diagnosticsJobStatusAllHandler(w http.ResponseWriter, _ *http.Request) {
	status, err := h.job.getStatusAll()
	if err != nil {
		response, _ := prepareResponseWithErr(http.StatusServiceUnavailable, err)
		writeResponse(w, response)
		return
	}
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// A handler function cancels a job running on a local node first. If a job is running on a remote node
// it will try to send a POST request to cancel it.
func (h *handler) cancelBundleReportHandler(w http.ResponseWriter, _ *http.Request) {
	response, err := h.job.cancel()
	if err != nil {
		log.Errorf("Could not cancel a job: %s", err)
	}
	writeResponse(w, response)
}

// A handler function returns a map of master ip as a key and a list of bundles as a value.
func (h *handler) listAvailableGLobalBundlesFilesHandler(w http.ResponseWriter, _ *http.Request) {
	allBundles, err := listAllBundles(h.cfg, h.tools)
	if err != nil {
		response, _ := prepareResponseWithErr(http.StatusServiceUnavailable, err)
		writeResponse(w, response)
		return
	}
	if err := json.NewEncoder(w).Encode(allBundles); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// A handler function returns a list of URLs to download bundles
func (h *handler) listAvailableLocalBundlesFilesHandler(w http.ResponseWriter, _ *http.Request) {
	matches, err := h.job.findLocalBundle()
	if err != nil {
		response, _ := prepareResponseWithErr(http.StatusServiceUnavailable, err)
		writeResponse(w, response)
		return
	}

	var localBundles []bundle
	for _, file := range matches {
		baseFile := filepath.Base(file)
		fileInfo, err := os.Stat(file)
		if err != nil {
			log.Errorf("Could not stat %s: %s", file, err)
			continue
		}

		localBundles = append(localBundles, bundle{
			File: fmt.Sprintf("%s/report/diagnostics/serve/%s", baseRoute, baseFile),
			Size: fileInfo.Size(),
		})
	}
	if err := json.NewEncoder(w).Encode(localBundles); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// A handler function serves a static local file. If a file not available locally but
// available on a different node, it will do a reverse proxy.
func (h *handler) downloadBundleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node, location, ok, err := h.job.isBundleAvailable(vars["file"])
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	// check if the file is available on localhost.
	serveFile := h.cfg.FlagDiagnosticsBundleDir + "/" + vars["file"]
	_, err = os.Stat(serveFile)
	if err == nil {
		w.Header().Add("Content-disposition", fmt.Sprintf("attachment; filename=%s", vars["file"]))
		http.ServeFile(w, r, serveFile)
		return
	}

	// proxy to appropriate host with a file.
	scheme := "http"
	if h.cfg.FlagForceTLS {
		scheme = "https"
	}

	director := func(req *http.Request) {
		req.URL.Scheme = scheme
		req.URL.Host = net.JoinHostPort(node, strconv.Itoa(h.cfg.FlagMasterPort))
		req.URL.Path = location
	}
	proxy := &httputil.ReverseProxy{
		Director:  director,
		Transport: h.job.Transport,
	}
	proxy.ServeHTTP(w, r)
}

// A handler function to start a diagnostics job.
func (h *handler) createBundleHandler(w http.ResponseWriter, r *http.Request) {
	var req bundleCreateRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		response, _ := prepareResponseWithErr(http.StatusBadRequest, err)
		writeResponse(w, response)
		return
	}
	response, err := h.job.run(req)
	if err != nil {
		log.Errorf("Could not run a diagnostics job: %s", err)
	}
	writeCreateResponse(w, response)
}

// A handler function to to get a list of available logs on a node.
func (h *handler) logsListHandler(w http.ResponseWriter, _ *http.Request) {
	endpoints, err := h.job.getLogsEndpoints()
	if err != nil {
		response, _ := prepareResponseWithErr(http.StatusServiceUnavailable, err)
		writeResponse(w, response)
		return
	}
	if err := json.NewEncoder(w).Encode(endpoints); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

// return a log for past N hours for a specific systemd Unit
func (h *handler) getUnitLogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	timeout := time.Duration(h.cfg.FlagCommandExecTimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	unitLogOut, err := h.job.dispatchLogs(ctx, vars["provider"], vars["entity"])
	if err != nil {
		response, _ := prepareResponseWithErr(http.StatusServiceUnavailable, err)
		writeResponse(w, response)
		return
	}
	defer unitLogOut.Close()

	log.Infof("Start read %s", vars["entity"])
	io.Copy(w, unitLogOut)
	log.Infof("Done read %s", vars["entity"])
}

func (h *handler) selfTestHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(runSelfTest()); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

func httpError(w http.ResponseWriter, msg string, code int) {
	log.WithField("Code", code).Error(msg)
	http.Error(w, msg, code)
}

// A helper function to send a response.
func writeResponse(w http.ResponseWriter, response diagnosticsReportResponse) {
	w.WriteHeader(response.ResponseCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}

func writeCreateResponse(w http.ResponseWriter, response createResponse) {
	w.WriteHeader(response.ResponseCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode responses to json: %s", err)
	}
}
