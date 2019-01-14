package api

import (
	"errors"

	"github.com/dcos/dcos-diagnostics/dcos"
)

// CheckUnitHealth returns an error on darwin because it's not supported
func (u *UnitPropertiesResponse) CheckUnitHealth() (dcos.Health, string, error) {
	return dcos.Unhealthy, "", errors.New("does not work on darwin")
}
