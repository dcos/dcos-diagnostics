package api

import (
	"fmt"
	"testing"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/stretchr/testify/assert"
)

func TestUnitPropertiesResponse_CheckUnitHealth(t *testing.T) {
	tests := []struct {
		name   string
		in     UnitPropertiesResponse
		health dcos.Health
		info   string
		err    error
	}{
		{
			name:   "no data",
			health: dcos.Unhealthy,
			err:    fmt.Errorf("LoadState: , ActiveState:  and SubState:  must be set"),
		},
		{
			name:   "unknown state service",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "unknown", SubState: "dead"},
			health: dcos.Unhealthy,
			info:   "dcos-telegraf.socket state is not one of the possible states [active inactive activating]. Current state is [ unknown ]. Please check `systemctl show all dcos-telegraf.socket` to check current Unit state. ",
		},
		{
			name:   "not loaded",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "not loaded", ActiveState: "unknown", SubState: "dead"},
			health: dcos.Unhealthy,
			info:   "dcos-telegraf.socket is not loaded. Please check `systemctl show all` to check current Unit status.",
		},
		{
			name:   "exit status not 0",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "inactive", SubState: "dead", ExecMainStatus: 1},
			health: dcos.Unhealthy,
			info:   "ExecMainStatus return failed status for dcos-telegraf.socket",
		},
		{
			name:   "unit flapping never been able to switch to active state",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "activating", SubState: "auto-restart"},
			health: dcos.Unhealthy,
			info:   "Unit dcos-telegraf.socket has never entered `active` state",
		},
		{
			name:   "unit flapping but it was running some time ago",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "activating", SubState: "auto-restart", InactiveEnterTimestampMonotonic: 2, ActiveEnterTimestampMonotonic: 1},
			health: dcos.Unhealthy,
			info:   "Unit dcos-telegraf.socket is flapping. Please check `systemctl status dcos-telegraf.socket` to check current Unit state.",
		},
		{
			name:   "unit starting and able to switch to active state",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "activating", SubState: "auto-restart", ActiveEnterTimestampMonotonic: 1},
			health: dcos.Healthy,
		},
		{
			name:   "inactive service",
			in:     UnitPropertiesResponse{ID: "dcos-telegraf.socket", LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
			health: dcos.Healthy,
			info:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, i, err := tt.in.CheckUnitHealth()
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.health, h)
			assert.Equal(t, tt.info, i)
		})
	}
}
