package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/units"
	"github.com/dcos/dcos-diagnostics/util"

	"github.com/dcos/dcos-go/exec"
	"github.com/shirou/gopsutil/disk"
	"github.com/sirupsen/logrus"
)

const (
	// All stands for collecting logs from all discovered nodes.
	All = "all"

	// Masters stand for collecting from discovered master nodes.
	Masters = "masters"

	// Agents stand for collecting from discovered agent/agent_public nodes.
	Agents = "agents"
)

// DiagnosticsJob is the main structure for a logs collection job.
type DiagnosticsJob struct {
	sync.RWMutex
	cancelChan   chan bool
	logProviders logProviders
	client       *http.Client

	Cfg       *config.Config    `json:"-"`
	DCOSTools dcos.Tooler       `json:"-"`
	Transport http.RoundTripper `json:"-"`

	Running               bool          `json:"is_running"`
	Status                string        `json:"status"`
	Errors                []string      `json:"errors"`
	LastBundlePath        string        `json:"last_bundle_dir"`
	JobStarted            time.Time     `json:"job_started"`
	JobEnded              time.Time     `json:"job_ended"`
	JobDuration           time.Duration `json:"job_duration"`
	JobProgressPercentage float32       `json:"job_progress_percentage"`
}

type logProviders struct {
	HTTPEndpoints map[string]HTTPProvider
	LocalFiles    map[string]FileProvider
	LocalCommands map[string]CommandProvider
}

// diagnostics job response format
type diagnosticsReportResponse struct {
	ResponseCode int      `json:"response_http_code"`
	Version      int      `json:"version"`
	Status       string   `json:"status"`
	Errors       []string `json:"errors"`
}

type createResponse struct {
	diagnosticsReportResponse
	Extra struct {
		LastBundleFile string `json:"bundle_name"`
	} `json:"extra"`
}

// diagnostics job status format
type bundleReportStatus struct {
	// job related fields
	Running               bool     `json:"is_running"`
	Status                string   `json:"status"`
	Errors                []string `json:"errors"`
	LastBundlePath        string   `json:"last_bundle_dir"`
	JobStarted            string   `json:"job_started"`
	JobEnded              string   `json:"job_ended"`
	JobDuration           string   `json:"job_duration"`
	JobProgressPercentage float32  `json:"job_progress_percentage"`

	// config related fields
	DiagnosticBundlesBaseDir                 string `json:"diagnostics_bundle_dir"`
	DiagnosticsJobTimeoutMin                 int    `json:"diagnostics_job_timeout_min"`
	DiagnosticsUnitsLogsSinceHours           string `json:"journald_logs_since_hours"`
	DiagnosticsJobGetSingleURLTimeoutMinutes int    `json:"diagnostics_job_get_since_url_timeout_min"`
	CommandExecTimeoutSec                    int    `json:"command_exec_timeout_sec"`

	// metrics related
	DiskUsedPercent float64 `json:"diagnostics_partition_disk_usage_percent"`
}

// Create a bundle request structure, example:   {"nodes": ["all"]}
type bundleCreateRequest struct {
	Version int
	Nodes   []string
}

// start a diagnostics job
func (j *DiagnosticsJob) run(req bundleCreateRequest) (createResponse, error) {

	role, err := j.DCOSTools.GetNodeRole()
	if err != nil {
		return prepareCreateResponseWithErr(http.StatusServiceUnavailable, err)
	}

	if role == dcos.AgentRole || role == dcos.AgentPublicRole {
		return prepareCreateResponseWithErr(http.StatusBadRequest, errors.New("running diagnostics job on agent node is not implemented"))
	}

	isRunning, _, err := j.isRunning()
	if err != nil {
		return prepareCreateResponseWithErr(http.StatusServiceUnavailable, err)
	}
	if isRunning {
		return prepareCreateResponseWithErr(http.StatusConflict, errors.New("Job is already running"))
	}

	foundNodes, err := findRequestedNodes(req.Nodes, j.DCOSTools)
	if err != nil {
		return prepareCreateResponseWithErr(http.StatusServiceUnavailable, err)
	}
	logrus.Debugf("Found requested nodes: %x", foundNodes)

	// try to create directory for diagnostic bundles
	_, err = os.Stat(j.Cfg.FlagDiagnosticsBundleDir)
	if os.IsNotExist(err) {
		logrus.Infof("Directory: %s not found, attempting to create one", j.Cfg.FlagDiagnosticsBundleDir)
		if err := os.MkdirAll(j.Cfg.FlagDiagnosticsBundleDir, os.ModePerm); err != nil {
			j.Status = "Could not create directory: " + j.Cfg.FlagDiagnosticsBundleDir
			return prepareCreateResponseWithErr(http.StatusServiceUnavailable, errors.New(j.Status))
		}
	}

	// Null errors on every new run.
	j.Errors = nil

	t := time.Now()
	bundleName := fmt.Sprintf("bundle-%d-%02d-%02d-%d.zip", t.Year(), t.Month(), t.Day(), t.Unix())

	j.LastBundlePath = filepath.Join(j.Cfg.FlagDiagnosticsBundleDir, bundleName)
	j.Status = "Diagnostics job started, archive will be available at: " + j.LastBundlePath
	j.cancelChan = make(chan bool)
	j.JobStarted = time.Now()
	j.Running = true
	go j.runBackgroundJob(foundNodes)

	var r createResponse
	r.Extra.LastBundleFile = bundleName
	r.ResponseCode = http.StatusOK
	r.Version = config.APIVer
	r.Status = "Job has been successfully started"
	return r, nil
}

//
func (j *DiagnosticsJob) runBackgroundJob(nodes []dcos.Node) {
	defer j.stop()

	const jobFailedStatus = "Job failed"
	if len(nodes) == 0 {
		j.Lock()
		e := "Nodes length cannot be 0"
		j.Status = jobFailedStatus
		j.Errors = append(j.Errors, e)
		j.Unlock()
		return
	}
	logrus.Info("Started background job")

	// lets start a goroutine which will timeout background report job after a certain time.
	jobIsDone := make(chan bool)
	// make sure we always cancel a timeout goroutine when the report is finished.
	defer func() {
		jobIsDone <- true
	}()

	go func(jobIsDone chan bool, j *DiagnosticsJob) {
		select {
		case <-jobIsDone:
			return
		case <-time.After(time.Minute * time.Duration(j.Cfg.FlagDiagnosticsJobTimeoutMinutes)):
			errMsg := fmt.Sprintf("diagnostics job timedout after: %s", time.Since(j.JobStarted))
			j.Lock()
			j.Status = jobFailedStatus
			j.Errors = append(j.Errors, errMsg)
			j.Unlock()
			logrus.Error(errMsg)
			j.cancelChan <- true
			return
		}
	}(jobIsDone, j)

	// create a zip file
	zipfile, err := os.Create(j.LastBundlePath)
	if err != nil {
		j.Status = jobFailedStatus
		errMsg := fmt.Sprintf("Could not create zip file: %s", j.LastBundlePath)
		j.Errors = append(j.Errors, errMsg)
		logrus.Error(errMsg)
		return
	}
	defer zipfile.Close()

	zipWriter := zip.NewWriter(zipfile)
	defer zipWriter.Close()

	// summaryReport is a log of a diagnostics job
	summaryReport := new(bytes.Buffer)

	// place a summaryErrorsReport.txt in a zip archive which should provide info what failed during the logs collection.
	summaryErrorsReport := new(bytes.Buffer)
	defer func() {
		// add a summaryErrorsReport.txt file to a diagnostics bundle, if it's not empty
		if summaryErrorsReport.Len() > 0 {
			zipFile, err := zipWriter.Create("summaryErrorsReport.txt")
			if err != nil {
				j.Status = "Could not append a summaryErrorsReport.txt to a zip file"
				logrus.Errorf("%s: %s", j.Status, err)
				j.Errors = append(j.Errors, err.Error())
				return
			}

			_, err = io.Copy(zipFile, summaryErrorsReport)
			if err != nil {
				logrus.Errorf("Error writing the summaryErrorsReport: %s", err)
			}
		}

		// flush the summary report
		zipFile, err := zipWriter.Create("summaryReport.txt")
		if err != nil {
			j.Status = "Could not append a summaryReport.txt to a zip file"
			logrus.Errorf("%s: %s", j.Status, err)
			j.Errors = append(j.Errors, err.Error())
			return
		}
		_, err = io.Copy(zipFile, summaryReport)
		if err != nil {
			logrus.Errorf("Error writing summaryReport: %s", err)
		}
	}()

	// lock out reportJob structure
	j.Lock()
	defer j.Unlock()

	// reset counters
	j.JobDuration = 0
	j.JobProgressPercentage = 0

	// we already checked for nodes length, we should not get division by zero error at this point.
	percentPerNode := 100.0 / float32(len(nodes))
	for _, node := range nodes {
		updateSummaryReport("START collecting logs", node, "", summaryReport)
		endpoints, err := j.getNodeEndpoints(node)
		if err != nil {
			j.logError(err, node, summaryErrorsReport)
			j.JobProgressPercentage += percentPerNode
		}

		// add http endpoints
		err = j.getHTTPAddToZip(node, endpoints, zipWriter, summaryErrorsReport, summaryReport, percentPerNode)
		if err != nil {
			j.Errors = append(j.Errors, err.Error())

			// handle job cancel error
			if serr, ok := err.(diagnosticsJobCanceledError); ok {
				logrus.Errorf("Could not add diagnostics to zip file: %s", serr)
				j.LastBundlePath = ""
				if removeErr := os.Remove(zipfile.Name()); removeErr != nil {
					logrus.Errorf("Could not remove a bundle: %s", removeErr)
					j.Errors = append(j.Errors, removeErr.Error())
				}
				return
			}

			logrus.Errorf("Could not add a log to a bundle: %s", err)
			updateSummaryReport(err.Error(), node, err.Error(), summaryErrorsReport)
		}
		updateSummaryReport("STOP collecting logs", node, "", summaryReport)
	}
	j.JobProgressPercentage = 100
	if len(j.Errors) == 0 {
		j.Status = "Diagnostics job successfully finished"
	} else {
		j.Status = "Diagnostics job failed"
	}
}

func (j *DiagnosticsJob) getNodeEndpoints(node dcos.Node) (endpoints map[string]string, e error) {
	port, err := getPullPortByRole(j.Cfg, node.Role)
	if err != nil {
		e = fmt.Errorf("used incorrect role: %s", err)
		return nil, e
	}
	url := fmt.Sprintf("http://%s:%d%s/logs", node.IP, port, baseRoute)
	body, statusCode, err := j.DCOSTools.Get(url, time.Second*3)
	if err != nil {
		e := fmt.Errorf("could not get a list of logs, url: %s, status code %d: %s", url, statusCode, err)
		return nil, e
	}
	if err = json.Unmarshal(body, &endpoints); err != nil {
		e := fmt.Errorf("could not unmarshal a list of logs from %s: %s", url, err)
		return nil, e
	}
	if len(endpoints) == 0 {
		e := fmt.Errorf("no endpoints found, url %s", url)
		return nil, e
	}
	return endpoints, nil
}

// delete a bundle
func (j *DiagnosticsJob) delete(bundleName string) (response diagnosticsReportResponse, err error) {
	if !strings.HasPrefix(bundleName, "bundle-") || !strings.HasSuffix(bundleName, ".zip") {
		return prepareResponseWithErr(http.StatusBadRequest, errors.New("format allowed  bundle-*.zip"))
	}

	j.Lock()
	defer j.Unlock()

	// first try to locate a bundle on a local disk.
	bundlePath := filepath.Join(j.Cfg.FlagDiagnosticsBundleDir, bundleName)
	logrus.Debugf("Trying remove a bundle: %s", bundlePath)
	_, err = os.Stat(bundlePath)
	if err == nil {
		if err = os.Remove(bundlePath); err != nil {
			return prepareResponseWithErr(http.StatusServiceUnavailable, err)
		}
		msg := "Deleted " + bundlePath
		logrus.Infof(msg)
		return prepareResponseOk(http.StatusOK, msg)
	}

	node, _, ok, err := j.isBundleAvailable(bundleName)
	if err != nil {
		return prepareResponseWithErr(http.StatusServiceUnavailable, err)
	}
	if ok {
		url := fmt.Sprintf("http://%s:%d%s/report/diagnostics/delete/%s", node, j.Cfg.FlagMasterPort, baseRoute, bundleName)
		j.Status = "Attempting to delete a bundle on a remote host. POST " + url
		logrus.Debug(j.Status)
		timeout := time.Second * 5
		response, _, err := j.DCOSTools.Post(url, timeout)
		if err != nil {
			return prepareResponseWithErr(http.StatusServiceUnavailable, err)
		}
		// unmarshal a response from a remote node and return it back.
		var remoteResponse diagnosticsReportResponse
		if err = json.Unmarshal(response, &remoteResponse); err != nil {
			return prepareResponseWithErr(http.StatusServiceUnavailable, err)
		}
		j.Status = remoteResponse.Status
		return remoteResponse, nil
	}
	j.Status = "Bundle not found " + bundleName
	return prepareResponseOk(http.StatusNotFound, j.Status)
}

// isRunning returns if the diagnostics job is running, node the job is running on and error. If the node is empty
// string, then the job is running on a localhost.
func (j *DiagnosticsJob) isRunning() (bool, string, error) {
	// first check if the job is running on a localhost.
	if j.Running {
		return true, "", nil
	}

	// try to discover if the job is running on other masters.
	clusterDiagnosticsJobStatus, err := j.getStatusAll()
	if err != nil {
		return false, "", err
	}
	for node, status := range clusterDiagnosticsJobStatus {
		if status.Running {
			return true, node, nil
		}
	}

	// no running job found.
	return false, "", nil
}

// Collect all status reports from master nodes and return a map[master_ip] bundleReportStatus
// The function is used to get a job status on other nodes
func (j *DiagnosticsJob) getStatusAll() (map[string]bundleReportStatus, error) {
	masterNodes, err := j.DCOSTools.GetMasterNodes()
	if err != nil {
		return nil, err
	}

	statuses := make(map[string]bundleReportStatus, len(masterNodes))

	localIP, err := j.DCOSTools.DetectIP()
	if err != nil {
		logrus.WithError(err).Warn("Could not detect IP")
	} else {
		statuses[localIP] = j.getStatus()
	}

	for _, master := range masterNodes {
		if master.IP == localIP {
			continue
		}
		var status bundleReportStatus
		url := fmt.Sprintf("http://%s:%d%s/report/diagnostics/status", master.IP, j.Cfg.FlagMasterPort, baseRoute)
		body, _, err := j.DCOSTools.Get(url, time.Second*3)
		if err != nil {
			logrus.WithError(err).WithField("URL", url).Error("Could not get data")
			continue
		}
		err = json.Unmarshal(body, &status)
		if err != nil {
			logrus.WithError(err).WithField("IP", master.IP).Errorf("Could not determine job status for master")
			continue
		}
		statuses[master.IP] = status
	}
	if len(statuses) == 0 {
		return statuses, errors.New("could not determine wheather the diagnostics job is running or not")
	}
	return statuses, nil
}

// get a status report for a localhost
func (j *DiagnosticsJob) getStatus() bundleReportStatus {
	// use a temp var `used`, since disk.Usage panics if partition does not exist.
	var used float64
	cfg := j.Cfg
	usageStat, err := disk.Usage(cfg.FlagDiagnosticsBundleDir)
	if err == nil {
		used = usageStat.UsedPercent
	} else {
		logrus.Errorf("Could not get a disk usage %s: %s", cfg.FlagDiagnosticsBundleDir, err)
	}

	j.RLock()
	status := bundleReportStatus{
		Running:               j.Running,
		Status:                j.Status,
		Errors:                append([]string{}, j.Errors...),
		LastBundlePath:        j.LastBundlePath,
		JobStarted:            j.JobStarted.String(),
		JobEnded:              j.JobEnded.String(),
		JobDuration:           j.JobDuration.String(),
		JobProgressPercentage: j.JobProgressPercentage,

		DiagnosticBundlesBaseDir:                 cfg.FlagDiagnosticsBundleDir,
		DiagnosticsJobTimeoutMin:                 cfg.FlagDiagnosticsJobTimeoutMinutes,
		DiagnosticsJobGetSingleURLTimeoutMinutes: cfg.FlagDiagnosticsJobGetSingleURLTimeoutMinutes,
		DiagnosticsUnitsLogsSinceHours:           cfg.FlagDiagnosticsBundleUnitsLogsSinceString,
		CommandExecTimeoutSec:                    cfg.FlagCommandExecTimeoutSec,

		DiskUsedPercent: used,
	}
	j.RUnlock()
	return status
}

type diagnosticsJobCanceledError struct {
	msg string
}

func (d diagnosticsJobCanceledError) Error() string {
	return d.msg
}

// fetch an HTTP endpoint and append the output to a zip file.
func (j *DiagnosticsJob) getHTTPAddToZip(node dcos.Node, endpoints map[string]string, zipWriter *zip.Writer,
	summaryErrorsReport, summaryReport *bytes.Buffer, percentPerNode float32) error {
	percentPerURL := percentPerNode / float32(len(endpoints))

	for fileName, httpEndpoint := range endpoints {
		select {
		case _, ok := <-j.cancelChan:
			if ok {
				updateSummaryReport("Job canceled", node, "", summaryErrorsReport)
				updateSummaryReport("Job canceled", node, "", summaryReport)
				return diagnosticsJobCanceledError{
					msg: "Job canceled",
				}
			}

		default:
			logrus.Debugf("GET %s%s", node.IP, httpEndpoint)
		}

		j.Status = "GET " + node.IP + httpEndpoint
		updateSummaryReport("START "+j.Status, node, "", summaryReport)
		e := j.getDataToZip(node, httpEndpoint, j.client, fileName, zipWriter)
		updateSummaryReport("STOP "+j.Status, node, "", summaryReport)
		if e != nil {
			j.logError(e, node, summaryErrorsReport)
		}
		j.JobProgressPercentage += percentPerURL
	}
	return nil
}

func (j *DiagnosticsJob) getDataToZip(node dcos.Node, httpEndpoint string, client *http.Client, fileName string, zipWriter *zip.Writer) error {
	fullURL, err := util.UseTLSScheme("http://"+node.IP+httpEndpoint, j.Cfg.FlagForceTLS)
	if err != nil {
		e := fmt.Errorf("could not read force-tls flag: %s", err)
		return e
	}
	resp, err := get(client, fullURL)
	if err != nil {
		e := fmt.Errorf("could not get from url %s: %s", fullURL, err)
		return e
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		fileName += ".gz"
	}
	// put all logs in a `ip_role` folder
	zipFile, err := zipWriter.Create(filepath.Join(node.IP+"_"+node.Role, fileName))
	defer resp.Body.Close()
	if err != nil {
		e := fmt.Errorf("could not add %s to a zip archive: %s", fileName, err)
		return e
	}
	io.Copy(zipFile, resp.Body)
	return nil
}

func (j *DiagnosticsJob) logError(e error, node dcos.Node, summaryErrorsReport *bytes.Buffer) {
	j.Errors = append(j.Errors, e.Error())
	logrus.Error(e)
	updateSummaryReport(e.Error(), node, e.Error(), summaryErrorsReport)
}

func get(client *http.Client, url string) (*http.Response, error) {
	logrus.Debugf("Using URL %s to collect a log", url)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create a new HTTP request: %s", err)
	}
	request.Header.Add("Accept-Encoding", "gzip")

	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not fetch url %s: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unable to fetch %s. Return code %d", url, resp.StatusCode)
	}

	return resp, err
}

func prepareResponseOk(httpStatusCode int, okMsg string) (response diagnosticsReportResponse, err error) {
	response, _ = prepareResponseWithErr(httpStatusCode, nil)
	response.Status = okMsg
	return response, nil
}

func prepareResponseWithErr(httpStatusCode int, e error) (response diagnosticsReportResponse, err error) {
	response.Version = config.APIVer
	response.ResponseCode = httpStatusCode
	if e != nil {
		response.Status = e.Error()
	}
	return response, e
}

func prepareCreateResponseWithErr(httpStatusCode int, e error) (createResponse, error) {
	cr := createResponse{}
	cr.ResponseCode = httpStatusCode
	cr.Version = config.APIVer
	if e != nil {
		cr.Status = e.Error()
	}
	return cr, e
}

// cancel a running job
func (j *DiagnosticsJob) cancel() (response diagnosticsReportResponse, err error) {
	role, err := j.DCOSTools.GetNodeRole()
	if err != nil {
		// Just log the error. We can still try to cancel the job.
		logrus.Errorf("Could not detect node role: %s", err)
	}
	if role == dcos.AgentRole || role == dcos.AgentPublicRole {
		return prepareResponseWithErr(http.StatusServiceUnavailable, errors.New("canceling diagnostics job on agent node is not implemented"))
	}

	// return error if we could not find if the job is running or not.
	isRunning, node, err := j.isRunning()
	if err != nil {
		return response, err
	}

	if !isRunning {
		return prepareResponseWithErr(http.StatusServiceUnavailable, errors.New("Job is not running"))
	}
	// if node is empty, try to cancel a job on a localhost
	if node == "" {
		j.cancelChan <- true
		logrus.Debug("Cancelling a local job")
	} else {
		url := fmt.Sprintf("http://%s:%d%s/report/diagnostics/cancel", node, j.Cfg.FlagMasterPort, baseRoute)
		j.Status = "Attempting to cancel a job on a remote host. POST " + url
		logrus.Debug(j.Status)
		response, _, err := j.DCOSTools.Post(url, time.Duration(j.Cfg.FlagDiagnosticsJobGetSingleURLTimeoutMinutes)*time.Minute)
		if err != nil {
			return prepareResponseWithErr(http.StatusServiceUnavailable, err)
		}
		// unmarshal a response from a remote node and return it back.
		var remoteResponse diagnosticsReportResponse
		if err = json.Unmarshal(response, &remoteResponse); err != nil {
			return prepareResponseWithErr(http.StatusServiceUnavailable, err)
		}
		return remoteResponse, nil

	}
	return prepareResponseOk(http.StatusOK, "Attempting to cancel a job, please check job status.")
}

func (j *DiagnosticsJob) stop() {
	j.Lock()
	j.Running = false
	j.JobEnded = time.Now()
	j.JobDuration = time.Since(j.JobStarted)
	j.Unlock()
	logrus.Info("Job finished")
}

// get a list of all bundles across the cluster.
func listAllBundles(cfg *config.Config, DCOSTools dcos.Tooler) (map[string][]bundle, error) {
	collectedBundles := make(map[string][]bundle)
	masterNodes, err := DCOSTools.GetMasterNodes()
	if err != nil {
		return collectedBundles, err
	}
	for _, master := range masterNodes {
		var bundleUrls []bundle
		url := fmt.Sprintf("http://%s:%d%s/report/diagnostics/list", master.IP, cfg.FlagMasterPort, baseRoute)
		body, _, err := DCOSTools.Get(url, time.Second*3)
		if err != nil {
			logrus.Errorf("Could not HTTP GET %s: %s", url, err)
			continue
		}
		if err = json.Unmarshal(body, &bundleUrls); err != nil {
			logrus.Errorf("Could not unmarshal response from %s: %s", url, err)
			continue
		}
		collectedBundles[fmt.Sprintf("%s:%d", master.IP, cfg.FlagMasterPort)] = bundleUrls
	}
	return collectedBundles, nil
}

// check if a bundle is available on a cluster.
func (j *DiagnosticsJob) isBundleAvailable(bundleName string) (string, string, bool, error) {
	logrus.WithField("Bundle", bundleName).Infof("Trying to find a bundle locally")
	localBundles, err := j.findLocalBundle()
	logrus.WithField("localBundles", localBundles).Info("Get list of local bundles")
	if err == nil {
		for _, bundle := range localBundles {
			if filepath.Base(bundle) == bundleName {
				return "", "", true, nil
			}
		}
	}
	logrus.WithField("Bundle", bundleName).WithError(err).Info("Not found bundle locally")

	bundles, err := listAllBundles(j.Cfg, j.DCOSTools)
	if err != nil {
		return "", "", false, err
	}
	logrus.Infof("Trying to find a bundle %s on remote hosts", bundleName)
	for host, remoteBundles := range bundles {
		for _, remoteBundle := range remoteBundles {
			if bundleName == filepath.Base(remoteBundle.File) {
				logrus.Infof("Bundle %s found on a host: %s", bundleName, host)
				hostPort := strings.Split(host, ":")
				if len(hostPort) > 0 {
					return hostPort[0], remoteBundle.File, true, nil
				}
				return "", "", false, errors.New("Node must be ip:port. Got " + host)
			}
		}
	}
	return "", "", false, nil
}

// return a a list of bundles available on a localhost.
func (j *DiagnosticsJob) findLocalBundle() (bundles []string, err error) {
	matches, err := filepath.Glob(j.Cfg.FlagDiagnosticsBundleDir + "/bundle-*.zip")
	if err != nil {
		return bundles, err
	}
	for _, localBundle := range matches {
		// skip a bundle zip file if the job is running
		if localBundle == j.LastBundlePath && j.Running {
			logrus.Infof("Skipped listing %s, the job is running", localBundle)
			continue
		}
		bundles = append(bundles, localBundle)
	}

	return bundles, nil
}

func matchRequestedNodes(requestedNodes []string, masterNodes, agentNodes []dcos.Node) ([]dcos.Node, error) {
	var matchedNodes []dcos.Node
	clusterNodes := append(masterNodes, agentNodes...)
	if len(requestedNodes) == 0 || len(clusterNodes) == 0 {
		return matchedNodes, errors.New("Cannot match requested nodes to clusterNodes")
	}

	for _, requestedNode := range requestedNodes {
		if requestedNode == "" {
			continue
		}

		if requestedNode == All {
			return clusterNodes, nil
		}
		if requestedNode == Masters {
			matchedNodes = append(matchedNodes, masterNodes...)
		}
		if requestedNode == Agents {
			matchedNodes = append(matchedNodes, agentNodes...)
		}
		// try to find nodes by ip / mesos id
		for _, clusterNode := range clusterNodes {
			if requestedNode == clusterNode.IP || requestedNode == clusterNode.MesosID || requestedNode == clusterNode.Host {
				matchedNodes = append(matchedNodes, clusterNode)
			}
		}
	}
	if len(matchedNodes) > 0 {
		return matchedNodes, nil
	}
	return matchedNodes, fmt.Errorf("Requested nodes: %s not found", requestedNodes)
}

func findRequestedNodes(requestedNodes []string, tools dcos.Tooler) ([]dcos.Node, error) {
	masterNodes, err := tools.GetMasterNodes()
	if err != nil {
		logrus.WithError(err).Errorf("Could not get master nodes")
	}

	agentNodes, err := tools.GetAgentNodes()
	if err != nil {
		logrus.WithError(err).Errorf("Could not get agent nodes")
	}
	return matchRequestedNodes(requestedNodes, masterNodes, agentNodes)
}

func (j *DiagnosticsJob) getLogsEndpoints() (endpoints map[string]string, err error) {
	endpoints = make(map[string]string)

	currentRole, err := j.DCOSTools.GetNodeRole()
	if err != nil {
		return nil, fmt.Errorf("failed to get a current role for a cfg: %s", err)
	}

	port, err := getPullPortByRole(j.Cfg, currentRole)
	if err != nil {
		return endpoints, err
	}

	// http endpoints
	for fileName, httpEndpoint := range j.logProviders.HTTPEndpoints {
		// if a role wasn't detected, consider to load all endpoints from a cfg file.
		// if the role could not be detected or it is not set in a cfg file use the log endpoint.
		// do not use the role only if it is set, detected and does not match the role form a cfg.
		if !roleMatched(currentRole, httpEndpoint.Role) {
			continue
		}
		endpoints[fileName] = fmt.Sprintf(":%d%s", httpEndpoint.Port, httpEndpoint.URI)
	}

	// file endpoints
	for sanitizedLocation, file := range j.logProviders.LocalFiles {
		if !roleMatched(currentRole, file.Role) {
			continue
		}
		endpoints[file.Location] = fmt.Sprintf(":%d%s/logs/files/%s", port, baseRoute, sanitizedLocation)
	}

	// command endpoints
	for cmdKey, c := range j.logProviders.LocalCommands {
		if !roleMatched(currentRole, c.Role) {
			continue
		}
		if cmdKey != "" {
			endpoints[cmdKey] = fmt.Sprintf(":%d%s/logs/cmds/%s", port, baseRoute, cmdKey)
		}
	}
	return endpoints, nil
}

// Init will prepare diagnostics job, read config files etc.
func (j *DiagnosticsJob) Init() error {
	providers, err := loadProviders(j.Cfg, j.DCOSTools)
	if err != nil {
		return fmt.Errorf("could not init diagnostic job: %s", err)
	}
	// set JobProgressPercentage -1 means the job has never been executed
	j.JobProgressPercentage = -1
	j.logProviders = logProviders{
		HTTPEndpoints: make(map[string]HTTPProvider),
		LocalFiles:    make(map[string]FileProvider),
		LocalCommands: make(map[string]CommandProvider),
	}
	// set filename if not set, some endpoints might be named e.g., after corresponding unit
	for _, endpoint := range providers.HTTPEndpoints {
		fileName := fmt.Sprintf("%d-%s.json", endpoint.Port, util.SanitizeString(endpoint.URI))
		if endpoint.FileName != "" {
			fileName = endpoint.FileName
		}
		j.logProviders.HTTPEndpoints[fileName] = endpoint
	}

	// trim left "/" and replace all slashes with underscores.
	for _, fileProvider := range providers.LocalFiles {
		key := strings.Replace(strings.TrimLeft(fileProvider.Location, "/"), "/", "_", -1)
		j.logProviders.LocalFiles[key] = fileProvider
	}

	// sanitize command to use as filename
	for _, commandProvider := range providers.LocalCommands {
		if len(commandProvider.Command) > 0 {
			cmdWithArgs := strings.Join(commandProvider.Command, "_")
			trimmedCmdWithArgs := strings.Replace(cmdWithArgs, "/", "", -1)
			key := fmt.Sprintf("%s.output", trimmedCmdWithArgs)
			j.logProviders.LocalCommands[key] = commandProvider
		}
	}

	timeout := time.Minute * time.Duration(j.Cfg.FlagDiagnosticsJobGetSingleURLTimeoutMinutes)
	j.client = util.NewHTTPClient(timeout, j.Transport)

	return nil
}

func roleMatched(myRole string, roles []string) bool {
	// if a role is empty, that means it does not matter master or agent, always return true.
	if len(roles) == 0 {
		return true
	}
	return util.IsInList(myRole, roles)
}

func (j *DiagnosticsJob) dispatchLogs(ctx context.Context, provider, entity string) (r io.ReadCloser, err error) {
	myRole, err := j.DCOSTools.GetNodeRole()
	if err != nil {
		return r, fmt.Errorf("could not get a node role: %s", err)
	}

	if provider == "units" {
		endpoint, ok := j.logProviders.HTTPEndpoints[entity]
		if !ok {
			return r, errors.New("Not found " + entity)
		}
		canExecute := roleMatched(myRole, endpoint.Role)
		if !canExecute {
			return r, errors.New("Only DC/OS systemd units are available")
		}
		logrus.Debugf("dispatching a Unit %s", entity)
		return units.ReadJournalOutputSince(entity, j.Cfg.FlagDiagnosticsBundleUnitsLogsSinceString)
	}

	if provider == "files" {
		logrus.Debugf("dispatching a file %s", entity)
		fileProvider, ok := j.logProviders.LocalFiles[entity]
		if !ok {
			return r, errors.New("Not found " + entity)
		}
		canExecute := roleMatched(myRole, fileProvider.Role)
		if !canExecute {
			return r, errors.New("Not allowed to read a file")
		}
		logrus.Debugf("Found a file %s", fileProvider.Location)
		return os.Open(fileProvider.Location)
	}
	if provider == "cmds" {
		logrus.Debugf("dispatching a command %s", entity)
		cmdProvider, ok := j.logProviders.LocalCommands[entity]
		if !ok {
			return r, errors.New("Not found " + entity)
		}
		canExecute := roleMatched(myRole, cmdProvider.Role)
		if !canExecute {
			return r, errors.New("Not allowed to execute a command")
		}
		var args []string
		if len(cmdProvider.Command) > 1 {
			args = cmdProvider.Command[1:]
		}

		ce, err := exec.Run(ctx, cmdProvider.Command[0], args)
		if err != nil {
			return nil, err
		}
		return &execCloser{ce}, nil
	}
	return r, errors.New("Unknown provider " + provider)
}

// the summary report is a file added to a zip bundle file to track any errors occurred during collection logs.
func updateSummaryReport(prefix string, node dcos.Node, err string, r *bytes.Buffer) {
	r.WriteString(fmt.Sprintf("%s [%s] %s %s %s\n", time.Now().String(), prefix, node.IP, node.Role, err))
}

// implement a io.ReadCloser wrapper over dcos/exec
type execCloser struct {
	r io.Reader
}

func (e *execCloser) Read(b []byte) (int, error) {
	return e.r.Read(b)
}

func (e *execCloser) Close() error {
	return nil
}
