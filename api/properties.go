package api

import (
	"strings"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
)

// UnitPropertiesResponse is a structure to unmarshal dbus.GetunitProperties response
type UnitPropertiesResponse struct {
	ID             string `mapstructure:"Id"`
	LoadState      string
	ActiveState    string
	SubState       string
	Description    string
	ExecMainStatus int

	InactiveExitTimestampMonotonic  uint64
	ActiveEnterTimestampMonotonic   uint64
	ActiveExitTimestampMonotonic    uint64
	InactiveEnterTimestampMonotonic uint64
}

func normalizeProperty(unitProps map[string]interface{}, tools dcos.Tooler) (HealthResponseValues, error) {
	var (
		description, prettyName string
		propsResponse           UnitPropertiesResponse
	)

	if err := mapstructure.Decode(unitProps, &propsResponse); err != nil {
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
