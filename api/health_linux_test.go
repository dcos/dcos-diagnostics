package api

import (
	"testing"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/stretchr/testify/assert"
)

const (
	title = "My fake description"
	name  = "PrettyName"
)

func TestSystemdUnits_GetUnits(t *testing.T) {
	s := SystemdUnits{}

	units, err := s.GetUnits(&fakeDCOSTools{})

	assert.NoError(t, err)
	assert.Equal(t, []HealthResponseValues{
		{UnitID: "unit_a", UnitTitle: title, PrettyName: name},
		{UnitID: "unit_b", UnitTitle: title, PrettyName: name},
		{UnitID: "unit_c", UnitTitle: title, PrettyName: name},
	}, units)
}

func TestSystemdUnits_GetUnitsProperties(t *testing.T) {
	s := SystemdUnits{}

	units, err := s.GetUnitsProperties(&fakeDCOSTools{})
	assert.NoError(t, err)

	expected := UnitsHealthResponseJSONStruct{
		Hostname: "MyHostName", IPAddress: "127.0.0.1", DcosVersion: "", Role: "master", MesosID: "node-id-123", TdtVersion: "0.4.0",
		Array: []HealthResponseValues{
			{UnitID: "unit_a", UnitHealth: dcos.Unhealthy, UnitTitle: title, PrettyName: name},
			{UnitID: "unit_b", UnitHealth: dcos.Unhealthy, UnitTitle: title, PrettyName: name},
			{UnitID: "unit_c", UnitHealth: dcos.Unhealthy, UnitTitle: title, PrettyName: name},
		},
	}
	assert.Equal(t, expected, units)
}
