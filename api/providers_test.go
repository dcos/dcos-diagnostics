package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadCollectors(t *testing.T) {
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("master", nil)
	tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.NoError(t, err)
	assert.Len(t, got, 15)
	expected := []string{
		"dcos-diagnostics",
		"5050-master_state-summary.json",
		"5050-registrar_1__registry.json",
		"uri_not_avail.txt",
		"5050-system_stats_json.json",
		"dcos-diagnostics-health.json",
		"var_lib_dcos_exhibitor_zookeeper_snapshot_myid",
		"var_lib_dcos_exhibitor_conf_zoo.cfg",
		"not_existing_file",
		"dmesg_-T.output",
		"ps_aux_ww_Z.output",
		"binsh_-c_cat etc*-release.output",
		"systemctl_list-units_dcos*.output",
		"echo_OK.output",
		"does_not_exist.output",
	}

	for i, c := range got {
		assert.Equal(t, expected[i], c.Name())
	}
}

func TestLoadCollectors_GetNodeRoleErrors(t *testing.T) {
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("master", errors.New("some error"))
	tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "could not get role: some error")
	assert.Empty(t, got)
}

func TestLoadCollectors_GetUnitNamesErrors(t *testing.T) {
	tools := new(MockedTools)

	tools.On("GetUnitNames").Return([]string{}, errors.New("some error"))

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "could load systemd collectors: could not get unit names: some error")
	assert.Empty(t, got)
}

func TestLoadCollectors_GetNodeRoleReturnsInvalidRole(t *testing.T) {
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("invalid", nil)
	tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "incorrect role invalid, must be: master, agent or agent_public")
	assert.Empty(t, got)
}
