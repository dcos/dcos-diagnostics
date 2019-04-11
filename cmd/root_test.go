// Copyright © 2017 Mesosphere Inc. <http://mesosphere.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/stretchr/testify/assert"
)

func Test_initConfig(t *testing.T) {
	cfgFile = filepath.Join("testdata", "dcos-diagnostics-config.json")
	defer func() { cfgFile = "" }()

	initConfig()

	expected := &config.Config{
		FlagRole:                       "master",
		FlagPort:                       1050,
		FlagPull:                       true,
		FlagMasterPort:                 1050,
		FlagAgentPort:                  61001,
		FlagPullInterval:               60,
		FlagPullTimeoutSec:             3,
		FlagUpdateHealthReportInterval: 60,
		FlagExhibitorClusterStatusURL:  "http://127.0.0.1:8181/exhibitor/v1/cluster/status",
		FlagDisableUnixSocket:          true,
		FlagDiagnosticsBundleDir:       "diag-bundles",
		FlagDiagnosticsBundleEndpointsConfigFiles:    []string{"dcos-diagnostics-endpoint-config.json"},
		FlagDiagnosticsBundleUnitsLogsSinceString:    "24h",
		FlagDiagnosticsJobTimeoutMinutes:             720,
		FlagDiagnosticsJobGetSingleURLTimeoutMinutes: 2,
		FlagCommandExecTimeoutSec:                    120,
	}

	assert.Equal(t, expected, defaultConfig)

}


func Test_initConfig_multiple_endpints_configs(t *testing.T) {
	cfgFile = filepath.Join("testdata", "dcos-diagnostics-config-multiple-endpoints-configs.json")
	defer func() { cfgFile = "" }()

	initConfig()

	expected := &config.Config{
		FlagRole:                       "master",
		FlagPort:                       1050,
		FlagPull:                       true,
		FlagMasterPort:                 1050,
		FlagAgentPort:                  61001,
		FlagPullInterval:               60,
		FlagPullTimeoutSec:             3,
		FlagUpdateHealthReportInterval: 60,
		FlagExhibitorClusterStatusURL:  "http://127.0.0.1:8181/exhibitor/v1/cluster/status",
		FlagDisableUnixSocket:          true,
		FlagDiagnosticsBundleDir:       "diag-bundles",
		FlagDiagnosticsBundleEndpointsConfigFiles:    []string{"1", "2"},
		FlagDiagnosticsBundleUnitsLogsSinceString:    "24h",
		FlagDiagnosticsJobTimeoutMinutes:             720,
		FlagDiagnosticsJobGetSingleURLTimeoutMinutes: 2,
		FlagCommandExecTimeoutSec:                    120,
	}

	assert.Equal(t, expected, defaultConfig)

}
