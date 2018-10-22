package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsJobInitReturnsErrorWhenConfigurationIsInvalid(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}

	// file does not exist
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = "test_endpoints-config.json"

	err := job.Init()
	assert.Error(t, err) // we can't use ErrorEqual: system errors differ between unix and windows
	assert.Contains(t, err.Error(), "could not init diagnostic job: could not initialize external log providers: open test_endpoints-config.json:")

	// file exists but is not valid JSON
	tmpfile, err := ioutil.TempFile("", "test_endpoints-config.json")
	defer os.Remove(tmpfile.Name())
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = tmpfile.Name()

	err = job.Init()
	assert.EqualError(t, err, "could not init diagnostic job: could not initialize external log providers: unexpected end of JSON input")
}

func TestDiagnosticsJobInitWithValidFile(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	err := job.Init()
	assert.NoError(t, err)

	assert.Equal(t, float32(-1), job.JobProgressPercentage)
	httpProviders := map[string]HTTPProvider{
		"5050-__processes__.json":         {Port: 5050, URI: "/__processes__", Role: []string{"master"}},
		"5050-master_state-summary.json":  {Port: 5050, URI: "/master/state-summary", Role: []string{"master"}},
		"5050-registrar_1__registry.json": {Port: 5050, URI: "/registrar(1)/registry", Role: []string{"master"}},
		"5050-system_stats_json.json":     {Port: 5050, URI: "/system/stats.json", Role: []string{"master"}},
		"5051-__processes__.json":         {Port: 5051, URI: "/__processes__", Role: []string{"agent", "agent_public"}},
		"5051-metrics_snapshot.json":      {Port: 5051, URI: "/metrics/snapshot", Role: []string{"agent", "agent_public"}},
		"5051-system_stats_json.json":     {Port: 5051, URI: "/system/stats.json", Role: []string{"agent", "agent_public"}},
		"dcos-diagnostics-health.json":    {Port: 1050, URI: "/system/health/v1", FileName: "dcos-diagnostics-health.json"},
		"dcos-download.service":           {Port: 1050, URI: "/system/health/v1/logs/units/dcos-download.service", FileName: "dcos-download.service"},
		"dcos-link-env.service":           {Port: 1050, URI: "/system/health/v1/logs/units/dcos-link-env.service", FileName: "dcos-link-env.service"},
		"dcos-setup.service":              {Port: 1050, URI: "/system/health/v1/logs/units/dcos-setup.service", FileName: "dcos-setup.service"},
		"unit_a":                          {Port: 1050, URI: "/system/health/v1/logs/units/unit_a", FileName: "unit_a"},
		"unit_b":                          {Port: 1050, URI: "/system/health/v1/logs/units/unit_b", FileName: "unit_b"},
		"unit_c":                          {Port: 1050, URI: "/system/health/v1/logs/units/unit_c", FileName: "unit_c"},
		"unit_to_fail":                    {Port: 1050, URI: "/system/health/v1/logs/units/unit_to_fail", FileName: "unit_to_fail"},
	}

	assert.Equal(t, httpProviders, job.logProviders.HTTPEndpoints)
	assert.Equal(t, map[string]FileProvider{
		"etc_resolv.conf": {Location: "/etc/resolv.conf"},
		"opt_mesosphere_active.buildinfo.full.json":      {Location: "/opt/mesosphere/active.buildinfo.full.json"},
		"var_lib_dcos_exhibitor_conf_zoo.cfg":            {Location: "/var/lib/dcos/exhibitor/conf/zoo.cfg", Role: []string{"master"}},
		"var_lib_dcos_exhibitor_zookeeper_snapshot_myid": {Location: "/var/lib/dcos/exhibitor/zookeeper/snapshot/myid", Role: []string{"master"}},
	}, job.logProviders.LocalFiles)

	assert.Equal(t, map[string]CommandProvider{
		"binsh_-c_cat etc*-release-3.output": {Command: []string{"/bin/sh", "-c", "cat /etc/*-release"}},
		"dmesg_-T-0.output":                  {Command: []string{"dmesg", "-T"}},
		"echo_OK-5.output":                   {Command: []string{"echo", "OK"}},
		"optmesospherebincurl_-s_-S_http:localhost:62080v1vips-2.output": {
			Command: []string{"/opt/mesosphere/bin/curl", "-s", "-S", "http://localhost:62080/v1/vips"},
			Role:    []string{"agent", "agent_public"},
		},
		"ps_aux_ww_Z-1.output":                {Command: []string{"ps", "aux", "ww", "Z"}},
		"systemctl_list-units_dcos*-4.output": {Command: []string{"systemctl", "list-units", "dcos*"}},
	}, job.logProviders.LocalCommands)

}

func TestGetLogsEndpoints(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	err := job.Init()
	require.NoError(t, err)

	endpoints, err := job.getLogsEndpoints()
	assert.NoError(t, err)

	const logPath = ":1050/system/health/v1/logs/"
	assert.Equal(t, endpoints, map[string]string{
		"/etc/resolv.conf":                                logPath + "files/etc_resolv.conf",
		"/opt/mesosphere/active.buildinfo.full.json":      logPath + "files/opt_mesosphere_active.buildinfo.full.json",
		"/var/lib/dcos/exhibitor/conf/zoo.cfg":            logPath + "files/var_lib_dcos_exhibitor_conf_zoo.cfg",
		"/var/lib/dcos/exhibitor/zookeeper/snapshot/myid": logPath + "files/var_lib_dcos_exhibitor_zookeeper_snapshot_myid",
		"5050-__processes__.json":                         ":5050/__processes__",
		"5050-master_state-summary.json":                  ":5050/master/state-summary",
		"5050-registrar_1__registry.json":                 ":5050/registrar(1)/registry",
		"5050-system_stats_json.json":                     ":5050/system/stats.json",
		"binsh_-c_cat etc*-release-3.output":              logPath + "cmds/binsh_-c_cat etc*-release-3.output",
		"dcos-diagnostics-health.json":                    ":1050/system/health/v1",
		"dcos-download.service":                           logPath + "units/dcos-download.service",
		"dcos-link-env.service":                           logPath + "units/dcos-link-env.service",
		"dcos-setup.service":                              logPath + "units/dcos-setup.service",
		"dmesg_-T-0.output":                               logPath + "cmds/dmesg_-T-0.output",
		"echo_OK-5.output":                                logPath + "cmds/echo_OK-5.output",
		"ps_aux_ww_Z-1.output":                            logPath + "cmds/ps_aux_ww_Z-1.output",
		"systemctl_list-units_dcos*-4.output":             logPath + "cmds/systemctl_list-units_dcos*-4.output",
		"unit_a":                                          logPath + "units/unit_a",
		"unit_b":                                          logPath + "units/unit_b",
		"unit_c":                                          logPath + "units/unit_c",
		"unit_to_fail":                                    logPath + "units/unit_to_fail",
	}, "only endpoints for master role should appear here")
}

func TestDispatchLogsForCommand(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	err := job.Init()
	require.NoError(t, err)

	r, err := job.dispatchLogs(context.TODO(), "cmds", "echo_OK-5.output")
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "OK\n", string(data))
}

func TestDispatchLogsForFiles(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	_, err = f.WriteString("OK")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	job.logProviders.LocalFiles = map[string]FileProvider{"ok": {Location: f.Name()}}

	r, err := job.dispatchLogs(context.TODO(), "files", "ok")
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "OK", string(data))
}

func TestDispatchLogsForUnit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	err := job.Init()
	require.NoError(t, err)

	r, err := job.dispatchLogs(context.TODO(), "units", "unit_a")
	assert.NoError(t, err)

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestDispatchLogsForUnit_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	job.Cfg.FlagDiagnosticsBundleEndpointsConfigFile = filepath.Join("testdata", "endpoint-config.json")

	err := job.Init()
	require.NoError(t, err)

	r, err := job.dispatchLogs(context.TODO(), "units", "unit_a")
	assert.Nil(t, r)
	assert.EqualError(t, err, "there is no journal on Windows")
}

func TestDispatchLogsWithUnknownProvider(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}

	r, err := job.dispatchLogs(context.TODO(), "unknown", "echo_OK-5.output")
	assert.EqualError(t, err, "Unknown provider unknown")
	assert.Nil(t, r)
}

func TestDispatchLogsWithUnknownEntity(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}

	for _, provider := range []string{"cmds", "files", "units"} {
		r, err := job.dispatchLogs(context.TODO(), provider, "unknown-entity")
		assert.EqualError(t, err, "Not found unknown-entity")
		assert.Nil(t, r)
	}
}

func TestGetHTTPAddToZip(t *testing.T) {
	job := DiagnosticsJob{Cfg: testCfg(), DCOSTools: &fakeDCOSTools{}}
	server, _ := stubServer("/ping", "pong")
	defer server.Close()

	zipFile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	defer os.Remove(zipFile.Name())

	zipWriter := zip.NewWriter(zipFile)

	summaryReport := new(bytes.Buffer)
	summaryErrorsReport := new(bytes.Buffer)

	endpoints := map[string]string{"ping": "/ping", "pong": "/ping", "not found": "/404"}
	node := dcos.Node{IP: server.URL[7:]} // strip http://

	err = job.getHTTPAddToZip(node, endpoints, zipWriter, summaryErrorsReport, summaryReport, 3)
	assert.NoError(t, err)

	assert.Equal(t, float32(3.0), job.JobProgressPercentage)
	assert.Len(t, job.Errors, 1, "one URL could not be fetched")

	assert.Contains(t, summaryReport.String(), "ping")
	assert.Contains(t, summaryReport.String(), "404")
	assert.Contains(t, summaryErrorsReport.String(), "/404. Return code 404")

	// validate zip file
	err = zipWriter.Close()
	require.NoError(t, err)

	reader, err := zip.OpenReader(zipFile.Name())
	require.NoError(t, err)

	assert.Len(t, reader.File, 2)
	for _, f := range reader.File {
		rc, err := f.Open()
		require.NoError(t, err)

		data, err := ioutil.ReadAll(rc)
		require.NoError(t, err)

		assert.Equal(t, "pong\n", string(data))
	}
}

// http://keighl.com/post/mocking-http-responses-in-golang/
func stubServer(uri string, body string) (*httptest.Server, *http.Transport) {
	return mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() == uri {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, body)
		} else {
			w.WriteHeader(404)
		}
	})
}

func mockServer(handle func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *http.Transport) {
	server := httptest.NewServer(http.HandlerFunc(handle))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return server, transport
}

func TestFindRequestedNodes(t *testing.T) {
	tools := new(MockedTools)

	tools.On("GetMasterNodes").Return(
		[]dcos.Node{
			{IP: "10.10.0.1", Role: "master"},
			{IP: "10.10.0.2", Host: "my-host.com", Role: "master"},
			{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"},
		}, nil)
	tools.On("GetAgentNodes").Return([]dcos.Node{{IP: "127.0.0.1", Role: "agent"}}, nil)

	var tests = []struct {
		requestedNodes []string
		expectedNodes  []dcos.Node
	}{
		{[]string{"all"}, []dcos.Node{
			{IP: "10.10.0.1", Role: "master"},
			{IP: "10.10.0.2", Role: "master", Host: "my-host.com"},
			{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"},
			{IP: "127.0.0.1", Role: "agent"},
		}},
		{[]string{"masters"}, []dcos.Node{
			{IP: "10.10.0.1", Role: "master"},
			{IP: "10.10.0.2", Role: "master", Host: "my-host.com"},
			{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"},
		}},
		{[]string{"agents"}, []dcos.Node{
			{IP: "127.0.0.1", Role: "agent"},
		}},
		{[]string{"10.10.0.1"}, []dcos.Node{
			{IP: "10.10.0.1", Role: "master"},
		}},
		{[]string{"my-host.com"}, []dcos.Node{
			{IP: "10.10.0.2", Role: "master", Host: "my-host.com"},
		}},
		{[]string{"12345-12345"}, []dcos.Node{
			{IP: "10.10.0.3", Role: "master", MesosID: "12345-12345"},
		}},
		{[]string{"agents", "10.10.0.1"}, []dcos.Node{
			{IP: "127.0.0.1", Role: "agent"},
			{IP: "10.10.0.1", Role: "master"},
		}},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.requestedNodes, "_"), func(t *testing.T) {
			actualNodes, err := findRequestedNodes(tt.requestedNodes, tools)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedNodes, actualNodes)
		})
	}

	tools.AssertExpectations(t)
}

func TestGetStatus(t *testing.T) {
	tools := &fakeDCOSTools{}
	config := testCfg()
	job := &DiagnosticsJob{Cfg: config, DCOSTools: tools}

	status := job.getStatus()
	assert.Equal(t, status.DiagnosticBundlesBaseDir, config.FlagDiagnosticsBundleDir)
}

func TestGetAllStatus(t *testing.T) {
	tools := &fakeDCOSTools{}
	config := testCfg()
	job := &DiagnosticsJob{Cfg: config, DCOSTools: tools}

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", baseRoute)
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

	status, err := job.getStatusAll()
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
	cfg := testCfg()
	defer os.RemoveAll(cfg.FlagDiagnosticsBundleDir)
	job := &DiagnosticsJob{Cfg: cfg, DCOSTools: tools}

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/list", baseRoute)
	mockedResponse := `[{"file_name": "/system/health/v1/report/diagnostics/serve/bundle-2016-05-13T22:11:36.zip", "file_size": 123}]`

	tools.makeMockedResponse(url, []byte(mockedResponse), http.StatusOK, nil)

	validFilePath := filepath.Join(cfg.FlagDiagnosticsBundleDir, "bundle-local.zip")
	_, err := os.Create(validFilePath)
	require.NoError(t, err)
	invalidFilePath := filepath.Join(cfg.FlagDiagnosticsBundleDir, "local-bundle.zip")
	_, err = os.Create(invalidFilePath)
	require.NoError(t, err)

	host, remoteSnapshot, ok, err := job.isBundleAvailable("bundle-2016-05-13T22:11:36.zip")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, host, "127.0.0.1")
	assert.Equal(t, remoteSnapshot, "/system/health/v1/report/diagnostics/serve/bundle-2016-05-13T22:11:36.zip")

	host, remoteSnapshot, ok, err = job.isBundleAvailable("bundle-local.zip")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, host)
	assert.Empty(t, remoteSnapshot)

	host, remoteSnapshot, ok, err = job.isBundleAvailable("local-bundle.zip")
	require.NoError(t, err)
	assert.False(t, ok, "bundles must mach bundle-*.zip pattern")
	assert.Empty(t, host)
	assert.Empty(t, remoteSnapshot)

	host, remoteSnapshot, ok, err = job.isBundleAvailable("bundle-123.zip")
	assert.False(t, ok)
	assert.Empty(t, host)
	assert.Empty(t, remoteSnapshot)
	require.NoError(t, err)
}

func TestCancelNotRunningJob(t *testing.T) {
	tools := &fakeDCOSTools{}
	dt := &Dt{
		Cfg:              testCfg(),
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", baseRoute)
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
		Cfg:              testCfg(),
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	// mock job status response
	url := "http://127.0.0.1:1050/system/health/v1/report/diagnostics/status/all"
	mockedResponse := `{"10.0.7.252":{"is_running":false}}`

	mockedMasters := []dcos.Node{
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
		Cfg:              testCfg(),
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools},
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
		Cfg:              testCfg(),
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools},
		MR:               &MonitoringResponse{},
	}
	router := NewRouter(dt)

	url := fmt.Sprintf("http://127.0.0.1:1050%s/report/diagnostics/status", baseRoute)
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

func TestDeleteBundleWithInvalidName(t *testing.T) {
	tools := &fakeDCOSTools{}
	job := &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools}

	response, err := job.delete("invalid name")

	assert.EqualError(t, err, "format allowed  bundle-*.zip")
	assert.Equal(t, diagnosticsReportResponse{
		ResponseCode: 400,
		Status:       "format allowed  bundle-*.zip",
		Version:      1,
	}, response)
}

func TestDeleteBundleWhenBundleNotFound(t *testing.T) {
	tools := &fakeDCOSTools{}
	job := &DiagnosticsJob{Cfg: testCfg(), DCOSTools: tools}

	response, err := job.delete("bundle-test.zip")

	assert.NoError(t, err)
	assert.Equal(t, diagnosticsReportResponse{
		ResponseCode: 404,
		Status:       "Bundle not found bundle-test.zip",
		Version:      1,
	}, response)
}

func TestDeleteBundleWhenBundleExistOnLocalNode(t *testing.T) {
	tools := &fakeDCOSTools{}
	cfg := testCfg()
	defer os.RemoveAll(cfg.FlagDiagnosticsBundleDir)
	job := &DiagnosticsJob{Cfg: cfg, DCOSTools: tools}

	bundlePath := filepath.Join(cfg.FlagDiagnosticsBundleDir, "bundle-test.zip")
	f, err := os.Create(bundlePath)
	require.NoError(t, err)
	f.Close()
	require.NoError(t, err)

	response, err := job.delete("bundle-test.zip")

	assert.NoError(t, err)
	assert.Equal(t, diagnosticsReportResponse{
		ResponseCode: 200,
		Status:       "Deleted " + bundlePath,
		Version:      1,
	}, response)
}

func TestRunSnapshot(t *testing.T) {
	tools := &fakeDCOSTools{}
	cfg := testCfg()
	defer os.RemoveAll(cfg.FlagDiagnosticsBundleDir)
	dt := &Dt{
		Cfg:              cfg,
		DtDCOSTools:      tools,
		DtDiagnosticsJob: &DiagnosticsJob{Cfg: cfg, DCOSTools: tools},
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
	// return empty list of endpoints
	tools.makeMockedResponse("http://127.0.0.1:1050/system/health/v1/logs", []byte("{}"), http.StatusOK, nil)

	body := bytes.NewBuffer([]byte(`{"nodes": ["all"]}`))
	response, code, _ := MakeHTTPRequest(t, router, "http://127.0.0.1:1050/system/health/v1/report/diagnostics/create", "POST", body)
	assert.Equal(t, http.StatusOK, code)
	var responseJSON createResponse
	err := json.Unmarshal(response, &responseJSON)
	assert.NoError(t, err)

	bundleRegexp := `^bundle-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{10}\.zip$`
	validBundleName := regexp.MustCompile(bundleRegexp)
	assert.True(t, validBundleName.MatchString(responseJSON.Extra.LastBundleFile),
		"invalid bundle name %s. Must match regexp: %s", responseJSON.Extra.LastBundleFile, bundleRegexp)

	assert.Equal(t, "Job has been successfully started", responseJSON.Status)
	assert.NotEmpty(t, responseJSON.Extra.LastBundleFile)

	bundle := filepath.Join(dt.Cfg.FlagDiagnosticsBundleDir, responseJSON.Extra.LastBundleFile)
	assert.True(t, waitForBundle(t, bundle))
}

func waitForBundle(t *testing.T, bundlePath string) bool {
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Log("Timeout!")
			return false
		default:
			stat, err := os.Stat(bundlePath)
			if err != nil {
				t.Logf("Error: %s", err)
				time.Sleep(time.Millisecond)
				continue
			}
			assert.False(t, stat.IsDir())
			return true
		}
	}
}
