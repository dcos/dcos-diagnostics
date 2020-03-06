package api

import (
	"errors"
	"os"
	"sync"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/sirupsen/logrus"
)

const dcosVersionEnvName = "DCOS_VERSION"

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

	// get all DC/OS Windows services
	foundUnits, err := tools.GetUnitNames()
	if err != nil {
		return nil, err
	}

	// get and check each units state
	for _, unit := range foundUnits {
		var normalizedProperty HealthResponseValues
		currentProperty, err := tools.GetUnitProperties(unit)
		if err != nil {
			logrus.Errorf("Could not get properties for Unit: %s", unit)
		} else {
			normalizedProperty, err = normalizeProperty(currentProperty, tools)
			if err != nil {
				logrus.Errorf("Could not normalize property for Unit %s: %s", unit, err)
				normalizedProperty.UnitID = ""
			}
		}
		if normalizedProperty.UnitID == "" {
			logrus.Errorf("Unit property error for %s", unit)
			normalizedProperty = HealthResponseValues{
				UnitID:     unit,
				UnitHealth: 1,
				UnitOutput: "",
				UnitTitle:  "",
				Help:       "",
				PrettyName: "",
			}
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

	healthReport.DcosVersion = os.Getenv(dcosVersionEnvName)
	if healthReport.DcosVersion == "" {
		var emptyReport UnitsHealthResponseJSONStruct
		return emptyReport, errors.New("DCOS_VERSION was not defined")
	}
	logrus.Infof("healthReport.DcosVersion = %s", healthReport.DcosVersion)

	healthReport.Role, err = tools.GetNodeRole()
	if err != nil {
		logrus.Errorf("Could not get node role: %s", err)
	}

	healthReport.MesosID, err = tools.GetMesosNodeID()
	if err != nil {
		logrus.Errorf("Could not get mesos node id: %s", err)
	}
	logrus.Infof("healthReport = %v", healthReport)
	return healthReport, nil
}
