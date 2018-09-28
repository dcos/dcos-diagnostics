package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	output = "The ActiveState is active, not in running state(4)"
	title  = "My fake description"
	name   = "PrettyName"
)

func TestSystemdUnits_GetUnits(t *testing.T) {
	s := SystemdUnits{}
	os.Setenv(dcosVersionEnvName, "some version")
	defer os.Unsetenv(dcosVersionEnvName)

	units, err := s.GetUnits(&fakeDCOSTools{})

	assert.NoError(t, err)
	expected := []HealthResponseValues{
		{UnitID: "dcos-setup.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "dcos-link-env.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "dcos-download.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_a", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_b", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_c", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_to_fail", UnitHealth: Healthy},
	}
	assert.Equal(t, expected, units)
}

func TestSystemdUnits_GetUnitsProperties(t *testing.T) {
	s := SystemdUnits{}
	os.Setenv(dcosVersionEnvName, "some version")
	defer os.Unsetenv(dcosVersionEnvName)

	units, err := s.GetUnitsProperties(&fakeDCOSTools{})
	assert.NoError(t, err)

	expected := UnitsHealthResponseJSONStruct{Array: []HealthResponseValues{
		{UnitID: "dcos-setup.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "dcos-link-env.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "dcos-download.service", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_a", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_b", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_c", UnitHealth: Healthy, UnitOutput: output, UnitTitle: title, PrettyName: name},
		{UnitID: "unit_to_fail", UnitHealth: Healthy},
	}, Hostname: "MyHostName", IPAddress: "127.0.0.1", DcosVersion: "some version", Role: "master", MesosID: "node-id-123", TdtVersion: "0.4.0"}

	assert.Equal(t, expected, units)
}
