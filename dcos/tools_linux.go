package dcos

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/coreos/go-systemd/dbus"
	"github.com/dcos/dcos-go/dcos/nodeutil"
	"github.com/dcos/dcos-log/dcos-log/journal/reader"
	"github.com/sirupsen/logrus"

	"github.com/dcos/dcos-diagnostics/units"
)

// Tools is implementation of Tooler interface.
type Tools struct {
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
func (st *Tools) InitializeUnitControllerConnection() (err error) {
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
func (st *Tools) CloseUnitControllerConnection() error {
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
func (st *Tools) GetUnitProperties(pname string) (map[string]interface{}, error) {
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
func (st *Tools) GetUnitNames() (units []string, err error) {
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
func (st *Tools) GetJournalOutput(unit string) (string, error) {
	matches := units.DefaultSystemdMatches(unit)
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
