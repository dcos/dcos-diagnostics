package api

import (
	"fmt"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc"
)

// CheckUnitHealth tells if the Unit is Healthy
func (u *UnitPropertiesResponse) CheckUnitHealth() (dcos.Health, string, error) {
	if u.ActiveState != fmt.Sprint(svc.Running) {
		logrus.Infof("The ActiveState is %s, not in running state(4)", u.ActiveState)
		return dcos.Unhealthy, fmt.Sprintf("The ActiveState is %s, not in running state(4)", u.ActiveState), nil
	}
	return dcos.Healthy, "", nil
}
