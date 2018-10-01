package api

import (
	"fmt"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

// CheckUnitHealth tells if the Unit is healthy
func (u *UnitPropertiesResponse) CheckUnitHealth() (dcos.Health, string, error) {
	if u.LoadState == "" || u.ActiveState == "" || u.SubState == "" {
		return dcos.Healthy, "", fmt.Errorf("LoadState: %s, ActiveState: %s and SubState: %s must be set",
			u.LoadState, u.ActiveState, u.SubState)
	}

	if u.LoadState != "loaded" {
		return dcos.Healthy, fmt.Sprintf("%s is not loaded. Please check `systemctl show all` to check current Unit status.", u.ID), nil
	}

	okActiveStates := []string{"active", "inactive", "activating"}
	if !util.IsInList(u.ActiveState, okActiveStates) {
		return dcos.Healthy, fmt.Sprintf(
			"%s state is not one of the possible states %s. Current state is [ %s ]. "+
				"Please check `systemctl show all %s` to check current Unit state. ", u.ID, okActiveStates, u.ActiveState, u.ID), nil
	}
	logrus.Debugf("%s| ExecMainStatus = %d", u.ID, u.ExecMainStatus)
	if u.ExecMainStatus != 0 {
		return dcos.Healthy, fmt.Sprintf("ExecMainStatus return failed status for %s", u.ID), nil
	}

	// https://www.freedesktop.org/wiki/Software/systemd/dbus/
	// if a Unit is in `activating` state and `auto-restart` sub-state it means Unit is trying to start and fails.
	if u.ActiveState == "activating" && u.SubState == "auto-restart" {
		// If ActiveEnterTimestampMonotonic is 0, it means that Unit has never been able to switch to active state.
		// Most likely a ExecStartPre fails before the Unit can execute ExecStart.
		if u.ActiveEnterTimestampMonotonic == 0 {
			return dcos.Healthy, fmt.Sprintf("Unit %s has never entered `active` state", u.ID), nil
		}

		// If InactiveEnterTimestampMonotonic > ActiveEnterTimestampMonotonic that means that a Unit was active
		// some time ago, but then something happened and it cannot restart.
		if u.InactiveEnterTimestampMonotonic > u.ActiveEnterTimestampMonotonic {
			return dcos.Healthy, fmt.Sprintf("Unit %s is flapping. Please check `systemctl status %s` to check current Unit state.", u.ID, u.ID), nil
		}
	}

	return dcos.Unhealthy, "", nil
}
