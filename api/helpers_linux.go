package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/dbus"
	"github.com/dcos/dcos-go/dcos/nodeutil"
	"github.com/dcos/dcos-log/dcos-log/journal/reader"
	"github.com/sirupsen/logrus"
)

// DCOSTools is implementation of DCOSHelper interface.
type DCOSTools struct {
	sync.Mutex

	ExhibitorURL string
	Role         string
	ForceTLS     bool
	NodeInfo     nodeutil.NodeInfo
	Transport    http.RoundTripper

	dcon     *dbus.Conn
	hostname string
}

// InitializeUnitControllerConnection opens a dbus connection. The connection is available via st.dcon
func (st *DCOSTools) InitializeUnitControllerConnection() (err error) {
	// we need to lock the dbus connection for each request
	st.Lock()
	if st.dcon == nil {
		st.dcon, err = dbus.New()
		if err != nil {
			st.Unlock()
			return err
		}
		return nil
	}
	st.Unlock()
	return errors.New("dbus connection is already opened")
}

// CloseUnitControllerConnection closes a dbus connection.
func (st *DCOSTools) CloseUnitControllerConnection() error {
	// unlock the dbus connection no matter what
	defer st.Unlock()
	if st.dcon != nil {
		st.dcon.Close()
		// since dbus api does not provide a way to check that the connection is closed, we'd nil it.
		st.dcon = nil
		return nil
	}
	return errors.New("dbus connection is closed")
}

// GetUnitProperties return a map of systemd Unit properties received from dbus.
func (st *DCOSTools) GetUnitProperties(pname string) (map[string]interface{}, error) {
	result, err := st.dcon.GetUnitProperties(pname)
	if err != nil {
		return result, err
	}

	// Get Service property
	propSlice := strings.Split(pname, ".")
	if len(propSlice) != 2 {
		return result, fmt.Errorf("Unit name must be in the following format: unitName.Type, got: %s", pname)
	}

	// let's get service specific properties
	// https://www.freedesktop.org/wiki/Software/systemd/dbus/
	if propSlice[1] == "service" {
		// "ExecMainStatus" will tell us main process exit code
		p, err := st.dcon.GetServiceProperty(pname, "ExecMainStatus")
		if err != nil {
			return result, err
		}
		result[p.Name] = p.Value.Value()
	}
	return result, nil
}

// GetUnitNames read a directory /etc/systemd/system/dcos.target.wants and return a list of found systemd units.
func (st *DCOSTools) GetUnitNames() (units []string, err error) {
	files, err := ioutil.ReadDir("/etc/systemd/system/dcos.target.wants")
	if err != nil {
		return units, err
	}
	for _, f := range files {
		units = append(units, f.Name())
	}
	logrus.Debugf("List of units: %s", units)
	return units, nil
}

// GetJournalOutput returns last 50 lines of journald command output for a specific systemd Unit.
func (st *DCOSTools) GetJournalOutput(unit string) (string, error) {
	matches := defaultSystemdMatches(unit)
	format := reader.NewEntryFormatter("text/plain", false)
	j, err := reader.NewReader(format, reader.OptionMatchOR(matches), reader.OptionSkipPrev(50))
	if err != nil {
		return "", err
	}
	defer j.Journal.Close()

	entries, err := ioutil.ReadAll(j)
	if err != nil {
		return "", err
	}

	return string(entries), nil
}

func normalizeProperty(unitProps map[string]interface{}, tools DCOSHelper) (HealthResponseValues, error) {
	var (
		description, prettyName string
		propsResponse           UnitPropertiesResponse
	)

	marshaledPropsResponse, err := json.Marshal(unitProps)
	if err != nil {
		return HealthResponseValues{}, err
	}

	if err = json.Unmarshal(marshaledPropsResponse, &propsResponse); err != nil {
		return HealthResponseValues{}, err
	}

	unitHealth, unitOutput, err := propsResponse.CheckUnitHealth()
	if err != nil {
		return HealthResponseValues{}, err
	}

	if unitHealth > 0 {
		journalOutput, err := tools.GetJournalOutput(propsResponse.ID)
		if err == nil {
			unitOutput += "\n"
			unitOutput += journalOutput
		} else {
			logrus.Errorf("Could not read journalctl: %s", err)
		}
	}

	s := strings.Split(propsResponse.Description, ": ")
	if len(s) != 2 {
		description = strings.Join(s, " ")

	} else {
		prettyName, description = s[0], s[1]
	}

	return HealthResponseValues{
		UnitID:     propsResponse.ID,
		UnitHealth: unitHealth,
		UnitOutput: unitOutput,
		UnitTitle:  description,
		Help:       "",
		PrettyName: prettyName,
	}, nil
}

// CheckUnitHealth tells if the Unit is healthy
func (u *UnitPropertiesResponse) CheckUnitHealth() (int, string, error) {
	if u.LoadState == "" || u.ActiveState == "" || u.SubState == "" {
		return 1, "", fmt.Errorf("LoadState: %s, ActiveState: %s and SubState: %s must be set",
			u.LoadState, u.ActiveState, u.SubState)
	}

	if u.LoadState != "loaded" {
		return 1, fmt.Sprintf("%s is not loaded. Please check `systemctl show all` to check current Unit status.", u.ID), nil
	}

	okActiveStates := []string{"active", "inactive", "activating"}
	if !isInList(u.ActiveState, okActiveStates) {
		return 1, fmt.Sprintf(
			"%s state is not one of the possible states %s. Current state is [ %s ]. "+
				"Please check `systemctl show all %s` to check current Unit state. ", u.ID, okActiveStates, u.ActiveState, u.ID), nil
	}
	logrus.Debugf("%s| ExecMainStatus = %d", u.ID, u.ExecMainStatus)
	if u.ExecMainStatus != 0 {
		return 1, fmt.Sprintf("ExecMainStatus return failed status for %s", u.ID), nil
	}

	// https://www.freedesktop.org/wiki/Software/systemd/dbus/
	// if a Unit is in `activating` state and `auto-restart` sub-state it means Unit is trying to start and fails.
	if u.ActiveState == "activating" && u.SubState == "auto-restart" {
		// If ActiveEnterTimestampMonotonic is 0, it means that Unit has never been able to switch to active state.
		// Most likely a ExecStartPre fails before the Unit can execute ExecStart.
		if u.ActiveEnterTimestampMonotonic == 0 {
			return 1, fmt.Sprintf("Unit %s has never entered `active` state", u.ID), nil
		}

		// If InactiveEnterTimestampMonotonic > ActiveEnterTimestampMonotonic that means that a Unit was active
		// some time ago, but then something happened and it cannot restart.
		if u.InactiveEnterTimestampMonotonic > u.ActiveEnterTimestampMonotonic {
			return 1, fmt.Sprintf("Unit %s is flapping. Please check `systemctl status %s` to check current Unit state.", u.ID, u.ID), nil
		}
	}

	return 0, "", nil
}

func readJournalOutputSince(unit, sinceString string) (io.ReadCloser, error) {
	matches := defaultSystemdMatches(unit)
	duration, err := time.ParseDuration(sinceString)
	if err != nil {
		logrus.Errorf("Error parsing %s. Defaulting to 24 hours", sinceString)
		duration = time.Hour * 24
	}
	format := reader.NewEntryFormatter("text/plain", false)
	j, err := reader.NewReader(format, reader.OptionMatchOR(matches), reader.OptionSince(duration))
	if err != nil {
		return nil, err
	}

	return j, nil
}

// returns default reader.JournalEntryMatch for a given systemd unit.
func defaultSystemdMatches(unit string) []reader.JournalEntryMatch {
	return []reader.JournalEntryMatch{
		{
			Field: systemdUnitProperty,
			Value: unit,
		},
		{
			Field: unitProperty,
			Value: unit,
		},
	}
}
