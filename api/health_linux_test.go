package api

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	title = "My fake description"
	name = "PrettyName"
)

func TestSystemdUnits_GetUnits(t *testing.T) {
	s := SystemdUnits{}

	units, err := s.GetUnits(&fakeDCOSTools{})

	assert.NoError(t, err)
	assert.Equal(t, []HealthResponseValues{
		{UnitID: "unit_a", UnitTitle: "My fake description", PrettyName: "PrettyName"},
		{UnitID: "unit_b", UnitTitle: "My fake description", PrettyName: "PrettyName"},
		{UnitID: "unit_c", UnitTitle: "My fake description", PrettyName: "PrettyName"},
	}, units)
}

func TestSystemdUnits_GetUnitsProperties(t *testing.T) {
	s := SystemdUnits{}

	units, err := s.GetUnitsProperties(&fakeDCOSTools{})
	assert.NoError(t, err)

	expected := UnitsHealthResponseJSONStruct{
		Hostname: "MyHostName", IPAddress: "127.0.0.1", DcosVersion: "", Role: "master", MesosID: "node-id-123", TdtVersion: "0.4.0",
		Array: []HealthResponseValues{
			{UnitID: "unit_a", UnitHealth: Unhealthy, UnitTitle: title, PrettyName: name},
			{UnitID: "unit_b", UnitHealth: Unhealthy, UnitTitle: title, PrettyName: name},
			{UnitID: "unit_c", UnitHealth: Unhealthy, UnitTitle: title, PrettyName: name},
		},
	}
	assert.Equal(t, expected, units)
}

