package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindRequestedNodes(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}

	mockedGlobalMonitoringResponse := &MonitoringResponse{
		Nodes: map[string]Node{
			"10.10.0.1": {
				IP:   "10.10.0.1",
				Role: "master",
			},
			"10.10.0.2": {
				IP:   "10.10.0.2",
				Host: "my-host.com",
				Role: "master",
			},
			"10.10.0.3": {
				IP:      "10.10.0.3",
				MesosID: "12345-12345",
				Role:    "master",
			},
			"127.0.0.1": {
				IP:   "127.0.0.1",
				Role: "agent",
			},
		},
	}
	dt.MR.UpdateMonitoringResponse(mockedGlobalMonitoringResponse)

	// should return masters + agents
	requestedNodes := []string{"all"}
	nodes, err := findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 4)
	assert.Contains(t, nodes, Node{IP: "10.10.0.1", Role: "master"})
	assert.Contains(t, nodes, Node{IP: "10.10.0.2", Role: "master", Host: "my-host.com"})
	assert.Contains(t, nodes, Node{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"})
	assert.Contains(t, nodes, Node{IP: "127.0.0.1", Role: "agent"})

	// should return only masters
	requestedNodes = []string{"masters"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 3)
	assert.Contains(t, nodes, Node{IP: "10.10.0.1", Role: "master"})
	assert.Contains(t, nodes, Node{IP: "10.10.0.2", Role: "master", Host: "my-host.com"})
	assert.Contains(t, nodes, Node{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"})

	// should return only agents
	requestedNodes = []string{"agents"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Contains(t, nodes, Node{IP: "127.0.0.1", Role: "agent"})

	// should return host with ip
	requestedNodes = []string{"10.10.0.1"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Contains(t, nodes, Node{IP: "10.10.0.1", Role: "master"})

	// should return host with hostname
	requestedNodes = []string{"my-host.com"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Contains(t, nodes, Node{IP: "10.10.0.2", Role: "master", Host: "my-host.com"})

	// should return host with mesos-id
	requestedNodes = []string{"12345-12345"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Contains(t, nodes, Node{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"})

	// should return agents and node with ip
	requestedNodes = []string{"agents", "10.10.0.1"}
	nodes, err = findRequestedNodes(requestedNodes, dt)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)
	assert.Contains(t, nodes, Node{IP: "10.10.0.1", Role: "master"})
	assert.Contains(t, nodes, Node{IP: "127.0.0.1", Role: "agent"})
}

func TestGetStatus(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}

	status := dt.DtDiagnosticsJob.getStatus()
	assert.Equal(t, status.DiagnosticBundlesBaseDir, DiagnosticsBundleDir)
}

func TestGetAllStatus(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", BaseRoute)
	mockedResponse := `
			{
			  "is_running":true,
			  "status":"MyStatus",
			  "errors":null,
			  "last_bundle_dir":"/path/to/snapshot",
			  "job_started":"0001-01-01 00:00:00 +0000 UTC",
			  "job_ended":"0001-01-01 00:00:00 +0000 UTC",
			  "job_duration":"2s",
			  "diagnostics_bundle_dir":"/home/core/1",
			  "diagnostics_job_timeout_min":720,
			  "diagnostics_partition_disk_usage_percent":28.0,
			  "journald_logs_since_hours": "24",
			  "diagnostics_job_get_since_url_timeout_min": 5,
			  "command_exec_timeout_sec": 10
			}
	`
	tools.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	status, err := dt.DtDiagnosticsJob.getStatusAll()
	require.NoError(t, err)
	assert.Contains(t, status, "127.0.0.1")
	assert.Equal(t, status["127.0.0.1"], bundleReportStatus{
		Running:                                  true,
		Status:                                   "MyStatus",
		LastBundlePath:                           "/path/to/snapshot",
		JobStarted:                               "0001-01-01 00:00:00 +0000 UTC",
		JobEnded:                                 "0001-01-01 00:00:00 +0000 UTC",
		JobDuration:                              "2s",
		DiagnosticBundlesBaseDir:                 "/home/core/1",
		DiagnosticsJobTimeoutMin:                 720,
		DiskUsedPercent:                          28.0,
		DiagnosticsUnitsLogsSinceHours:           "24",
		DiagnosticsJobGetSingleURLTimeoutMinutes: 5,
		CommandExecTimeoutSec:                    10,
	})
}

func TestIsSnapshotAvailable(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/list", BaseRoute)
	mockedResponse := `[{"file_name": "/system/health/v1/report/diagnostics/serve/bundle-2016-05-13T22:11:36.zip", "file_size": 123}]`

	tools.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	// should find
	host, remoteSnapshot, ok, err := dt.DtDiagnosticsJob.isBundleAvailable("bundle-2016-05-13T22:11:36.zip")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, host, "127.0.0.1")
	assert.Equal(t, remoteSnapshot, "/system/health/v1/report/diagnostics/serve/bundle-2016-05-13T22:11:36.zip")

	// should not find
	host, remoteSnapshot, ok, err = dt.DtDiagnosticsJob.isBundleAvailable("bundle-123.zip")
	assert.False(t, ok)
	assert.Empty(t, host)
	assert.Empty(t, remoteSnapshot)
	require.NoError(t, err)
}

func TestCancelNotRunningJob(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", BaseRoute)
	mockedResponse := `
			{
			  "is_running":false,
			  "status":"MyStatus",
			  "errors":null,
			  "last_bundle_dir":"/path/to/snapshot",
			  "job_started":"0001-01-01 00:00:00 +0000 UTC",
			  "job_ended":"0001-01-01 00:00:00 +0000 UTC",
			  "job_duration":"2s",
			  "diagnostics_bundle_dir":"/home/core/1",
			  "diagnostics_job_timeout_min":720,
			  "diagnostics_partition_disk_usage_percent":28.0,
			  "journald_logs_since_hours": "24",
			  "diagnostics_job_get_since_url_timeout_min": 5,
			  "command_exec_timeout_sec": 10
			}
	`
	st := &fakeDCOSTools{}
	st.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)
	dt.DtDCOSTools = st
	dt.DtDiagnosticsJob.DCOSTools = st

	// Job should fail because it is not running
	response, code, err := MakeHTTPRequest(t, router, "/system/health/v1/report/diagnostics/cancel", "POST", nil)
	assert.NoError(t, err)
	assert.Equal(t, code, http.StatusServiceUnavailable)
	var responseJSON diagnosticsReportResponse
	err = json.Unmarshal(response, &responseJSON)
	assert.NoError(t, err)
	assert.Equal(t, responseJSON, diagnosticsReportResponse{
		Version:      1,
		Status:       "Job is not running",
		ResponseCode: http.StatusServiceUnavailable,
	})
}

// Test we can cancel a job running on a different node.
func TestCancelGlobalJob(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	// mock job status response
	url := "http://127.0.0.1:1050/system/health/v1/report/diagnostics/status/all"
	mockedResponse := `{"10.0.7.252":{"is_running":false}}`

	mockedMasters := []Node{
		{
			Role: "master",
			IP:   "10.0.7.252",
		},
	}

	// add fake response for status/all
	st := &fakeDCOSTools{
		fakeMasters: mockedMasters,
	}
	st.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	// add fake response for status 10.0.7.252
	url = "http://10.0.7.252:1050/system/health/v1/report/diagnostics/status"
	mockedResponse = `
			{
			  "is_running":true,
			  "status":"MyStatus",
			  "errors":null,
			  "last_bundle_dir":"/path/to/snapshot",
			  "job_started":"0001-01-01 00:00:00 +0000 UTC",
			  "job_ended":"0001-01-01 00:00:00 +0000 UTC",
			  "job_duration":"2s",
			  "diagnostics_bundle_dir":"/home/core/1",
			  "diagnostics_job_timeout_min":720,
			  "diagnostics_partition_disk_usage_percent":28.0,
			  "journald_logs_since_hours": "24",
			  "diagnostics_job_get_since_url_timeout_min": 5,
			  "command_exec_timeout_sec": 10
			}
	`
	st.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)
	dt.DtDCOSTools = st
	dt.DtDiagnosticsJob.DCOSTools = st

	MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/cancel", "POST", nil)

	// if we have the url in f.postRequestsMade, that means the redirect worked correctly
	assert.Contains(t, st.postRequestsMade, "http://10.0.7.252:1050/system/health/v1/report/diagnostics/cancel")
}

// try cancel a local job
func TestCancelLocalJob(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	dt.DtDiagnosticsJob.Running = true
	dt.DtDiagnosticsJob.cancelChan = make(chan bool, 1)
	response, code, err := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/cancel", "POST", nil)
	assert.NoError(t, err)
	assert.Equal(t, code, http.StatusOK)

	var responseJSON diagnosticsReportResponse
	err = json.Unmarshal(response, &responseJSON)
	require.NoError(t, err)
	assert.Equal(t, responseJSON, diagnosticsReportResponse{
		Version:      1,
		Status:       "Attempting to cancel a job, please check job status.",
		ResponseCode: http.StatusOK,
	})
	r := <-dt.DtDiagnosticsJob.cancelChan
	assert.True(t, r)
}

func TestFailRunSnapshotJob(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", BaseRoute)
	mockedResponse := `
			{
			  "is_running":false,
			  "status":"MyStatus",
			  "errors":null,
			  "last_bundle_dir":"/path/to/snapshot",
			  "job_started":"0001-01-01 00:00:00 +0000 UTC",
			  "job_ended":"0001-01-01 00:00:00 +0000 UTC",
			  "job_duration":"2s",
			  "diagnostics_bundle_dir":"/home/core/1",
			  "diagnostics_job_timeout_min":720,
			  "diagnostics_partition_disk_usage_percent":28.0,
			  "journald_logs_since_hours": "24",
			  "diagnostics_job_get_since_url_timeout_min": 5,
			  "command_exec_timeout_sec": 10
			}
	`
	tools.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	// should fail since request is in wrong format
	body := bytes.NewBuffer([]byte(`{"nodes": "wrong"}`))
	_, code, _ := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/create", "POST", body)
	assert.Equal(t, code, http.StatusBadRequest)

	// node should not be found
	body = bytes.NewBuffer([]byte(`{"nodes": ["192.168.0.1"]}`))
	response, code, _ := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/create", "POST", body)
	assert.Equal(t, code, http.StatusServiceUnavailable)

	var responseJSON diagnosticsReportResponse
	err := json.Unmarshal(response, &responseJSON)
	require.NoError(t, err)
	assert.Equal(t, responseJSON.Status, "Requested nodes: [192.168.0.1] not found")
}

func TestRunSnapshot(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg, DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	url := "http://127.0.0.1:1050/system/health/v1/report/diagnostics/status"
	mockedResponse := `
			{
			  "is_running":false,
			  "status":"MyStatus",
			  "errors":null,
			  "last_bundle_dir":"/path/to/snapshot",
			  "job_started":"0001-01-01 00:00:00 +0000 UTC",
			  "job_ended":"0001-01-01 00:00:00 +0000 UTC",
			  "job_duration":"2s",
			  "diagnostics_bundle_dir":"/home/core/1",
			  "diagnostics_job_timeout_min":720,
			  "diagnostics_partition_disk_usage_percent":28.0,
			  "journald_logs_since_hours": "24",
			  "diagnostics_job_get_since_url_timeout_min": 5,
			  "command_exec_timeout_sec": 10
			}
	`
	tools.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	body := bytes.NewBuffer([]byte(`{"nodes": ["all"]}`))
	response, code, _ := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/create", "POST", body)
	assert.Equal(t, code, http.StatusOK)
	var responseJSON createResponse
	err := json.Unmarshal(response, &responseJSON)
	assert.NoError(t, err)

	bundle := DiagnosticsBundleDir + "/" + responseJSON.Extra.LastBundleFile
	defer func() {
		err := os.Remove(bundle)
		assert.NoError(t, err)
	}()

	bundleRegexp := `^bundle-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{10}\.zip$`
	validBundleName := regexp.MustCompile(bundleRegexp)
	assert.True(t, validBundleName.MatchString(responseJSON.Extra.LastBundleFile),
		"invalid bundle name %s. Must match regexp: %s", responseJSON.Extra.LastBundleFile, bundleRegexp)

	assert.Equal(t, responseJSON.Status, "Job has been successfully started")
	assert.NotEmpty(t, responseJSON.Extra.LastBundleFile)

	assert.True(t, waitForBundle(t, router))

	snapshotFiles, err := ioutil.ReadDir(DiagnosticsBundleDir)
	assert.NoError(t, err)
	assert.True(t, len(snapshotFiles) > 0)

	stat, err := os.Stat(bundle)
	assert.NoError(t, err)
	assert.False(t, stat.IsDir())
}

func waitForBundle(t *testing.T, router *mux.Router) bool {
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Log("Timeout!")
			return false
		default:
			response, code, err := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/status", "GET", nil)
			if err != nil {
				t.Logf("Error: %d", err)
				continue
			}
			if code != http.StatusOK {
				t.Logf("Invalid status code: %d", code)
				continue
			}
			diagnosticsJob := bundleReportStatus{}
			if err := json.Unmarshal(response, &diagnosticsJob); err != nil {
				t.Logf("Error when unmarshaling response: %s", err)
				continue
			}
			if diagnosticsJob.Running {
				t.Log("Bundle is not available. Retrying")
				continue
			}
			t.Logf("Bundle is available: %s", diagnosticsJob.LastBundlePath)
			return true
		}
	}
}
