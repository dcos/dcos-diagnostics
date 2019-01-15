package api

import (
	"errors"
	"sync"

	"github.com/dcos/dcos-diagnostics/dcos"
)

// SystemdUnits used to make GetUnitsProperties thread safe.
type SystemdUnits struct {
	sync.Mutex
}

// GetUnits returns an error on darwin because it's not supported
func (s *SystemdUnits) GetUnits(tools dcos.Tooler) (allUnits []HealthResponseValues, err error) {
	return nil, errors.New("does not work on darwin")
}

// GetUnitsProperties returns an error on darwin because it's not supported
func (s *SystemdUnits) GetUnitsProperties(tools dcos.Tooler) (healthReport UnitsHealthResponseJSONStruct, err error) {
	var emptyReport UnitsHealthResponseJSONStruct
	return emptyReport, errors.New("does not work on darwin")
}
