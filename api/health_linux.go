package api

import (
	"os"
	"sync"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"

	"github.com/sirupsen/logrus"
)

// SystemdUnits used to make GetUnitsProperties thread safe.
type SystemdUnits struct {
	sync.Mutex
}

// GetUnits returns a list of found unit properties.
func (s *SystemdUnits) GetUnits(tools dcos.Tooler) (allUnits []HealthResponseValues, err error) {
	if err = tools.InitializeUnitControllerConnection(); err != nil {
		return nil, err
	}
	defer tools.CloseUnitControllerConnection()

	// detect DC/OS systemd units
	foundUnits, err := tools.GetUnitNames()
	if err != nil {
		return nil, err
	}

	// DCOS-5862 blacklist systemd units
	excludeUnits := []string{"dcos-setup.service", "dcos-link-env.service", "dcos-download.service"}
	for _, unit := range foundUnits {
		if util.IsInList(unit, excludeUnits) {
			logrus.Debugf("Skipping blacklisted systemd Unit %s", unit)
			continue
		}
		currentProperty, err := tools.GetUnitProperties(unit)
		if err != nil {
			logrus.Errorf("Could not get properties for Unit: %s", unit)
			continue
		}
		normalizedProperty, err := normalizeProperty(currentProperty, tools)
		if err != nil {
			logrus.Errorf("Could not normalize property for Unit %s: %s", unit, err)
			continue
		}
		allUnits = append(allUnits, normalizedProperty)
	}
	return allUnits, nil
}

// GetUnitsProperties return a structured units health response of UnitsHealthResponseJsonStruct type.
func (s *SystemdUnits) GetUnitsProperties(tools dcos.Tooler) (healthReport UnitsHealthResponseJSONStruct, err error) {
	s.Lock()
	defer s.Unlock()

	healthReport.TdtVersion = config.Version
	healthReport.Hostname, err = tools.GetHostname()
	if err != nil {
		logrus.Errorf("Could not get a hostname: %s", err)
	}

	// update the rest of healthReport fields
	healthReport.Array, err = s.GetUnits(tools)
	if err != nil {
		logrus.Errorf("Unable to get a list of systemd units: %s", err)
	}

	healthReport.IPAddress, err = tools.DetectIP()
	if err != nil {
		logrus.Errorf("Could not detect IP: %s", err)
	}

	healthReport.DcosVersion = os.Getenv("DCOS_VERSION")
	healthReport.Role, err = tools.GetNodeRole()
	if err != nil {
		logrus.Errorf("Could not get node role: %s", err)
	}

	healthReport.MesosID, err = tools.GetMesosNodeID()
	if err != nil {
		logrus.Errorf("Could not get mesos node id: %s", err)
	}

	return healthReport, nil
}
