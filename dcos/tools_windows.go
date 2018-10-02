package dcos

import (
	"bufio"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dcos/dcos-go/dcos/nodeutil"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// WindowsServiceListFile has the expected service list, generated
	// during cluster deployment
	WindowsServiceListFile = "servicelist.txt"
)

// Tools is implementation of dcos.Tooler interface.
type Tools struct {
	sync.Mutex

	ExhibitorURL string
	Role         string
	ForceTLS     bool
	NodeInfo     nodeutil.NodeInfo
	Transport    http.RoundTripper

	svcManager *mgr.Mgr

	hostname string
	ip       string
	mesosID  string
}

// InitializeUnitControllerConnection opens a connection.
func (st *Tools) InitializeUnitControllerConnection() (err error) {
	st.Lock()
	if st.svcManager == nil {
		st.svcManager, err = mgr.Connect()
		if err != nil {
			st.Unlock()
			return err
		}
		return nil
	}
	st.Unlock()
	return errors.New("connection is already opened")
}

// CloseUnitControllerConnection closes a dbus connection.
func (st *Tools) CloseUnitControllerConnection() error {
	// unlock the connection no matter what
	defer st.Unlock()
	if st.svcManager != nil {
		err := st.svcManager.Disconnect()
		if err != nil {
			return err
		}
		st.svcManager = nil
		return nil
	}
	return errors.New("connection is closed")
}

// GetUnitProperties return a map of Windows service properties
func (st *Tools) GetUnitProperties(pname string) (map[string]interface{}, error) {
	var serviceHandle *mgr.Service

	if st.svcManager == nil {
		return nil, errors.New("connection was not opened")
	}

	// search for the target service
	serviceList, err := st.svcManager.ListServices()
	if err != nil {
		return nil, err
	}

	for _, service := range serviceList {
		if strings.Compare(pname, service) == 0 {
			serviceHandle, err = st.svcManager.OpenService(pname)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	if serviceHandle == nil {
		return nil, errors.New("service not found")
	}
	defer serviceHandle.Close()

	// get service config
	var config mgr.Config
	config, err = serviceHandle.Config()
	if err != nil {
		return nil, err
	}
	logrus.Debugf("config.DisplayName: [%s]", config.DisplayName)
	logrus.Debugf("config.Description: [%s]", config.Description)
	logrus.Debugf("config.BinaryPathName: [%s]", config.BinaryPathName)

	status, err := serviceHandle.Query()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	result["ID"] = pname
	result["ActiveState"] = string(status.State)
	result["LoadState"] = string(status.State)
	result["SubState"] = string(status.State)
	result["Description"] = config.Description

	logrus.WithField("Result", result).WithField("ID", pname).Debug("GetUnitProperties for service")
	return result, nil
}

// GetUnitNames reads from WindowsServiceListFile for a list of expected Windows services on the agent node
// In Windows, "units" are equivalent to Windows services
func (st *Tools) GetUnitNames() (units []string, err error) {
	// read all the Windows services from WindowsServiceListFile file
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	servicelistpath := exPath + "\\" + WindowsServiceListFile

	file, err := os.Open(servicelistpath)
	if err != nil {
		logrus.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		units = append(units, scanner.Text())
	}
	logrus.Infof("GetUnitNames: %s has %v", servicelistpath, units)
	return units, nil
}

// GetJournalOutput returns nil, as it's not supported on a Windwos agent node
func (st *Tools) GetJournalOutput(unit string) (string, error) {
	return "", nil
}
