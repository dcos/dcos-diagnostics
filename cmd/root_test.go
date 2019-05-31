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
	jwtt "github.com/spf13/jwalterweatherman"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const hostnameEnvVarName = "HOSTNAME"


func Test_initConfig(t *testing.T) {
	h := os.Getenv(hostnameEnvVarName)
	require.NoError(t, os.Unsetenv(hostnameEnvVarName), "remove hostname env to not override hostname from config file")
	defer func() {
		_ = os.Setenv(hostnameEnvVarName, h)
	}()

	cfgFile = filepath.Join("testdata", "dcos-diagnostics-config.json")
	defer func() { cfgFile = "" }()

	jwtt.SetStdoutThreshold(jwtt.LevelTrace)

	initConfig()

	expected := &config.Config{
		FlagRole:                       "master",
		FlagHostname:                   "master-0",
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
		FlagDiagnosticsBundleFetchersCount:           1,
	}

	assert.Equal(t, expected, defaultConfig)

}

func Test_initConfig_multiple_endpints_configs(t *testing.T) {
	h := os.Getenv(hostnameEnvVarName)
	require.NoError(t, os.Unsetenv(hostnameEnvVarName), "remove hostname env to not override hostname from config file")
	defer func() {
		_ = os.Setenv(hostnameEnvVarName, h)
	}()

	cfgFile = filepath.Join("testdata", "dcos-diagnostics-config-multiple-endpoints-configs.json")
	defer func() { cfgFile = "" }()

	jwtt.SetStdoutThreshold(jwtt.LevelTrace)

	initConfig()

	expected := &config.Config{
		FlagRole:                       "master",
		FlagHostname:                   "master-0",
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
		FlagDiagnosticsBundleFetchersCount:           1,
	}

	assert.Equal(t, expected, defaultConfig)

}

func TestPersistentPreRunSetsLogLevel(t *testing.T) {
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel())
	RootCmd.PersistentPreRun(nil, nil)
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel())

	defaultConfig.FlagVerbose = true
	RootCmd.PersistentPreRun(nil, nil)
	assert.Equal(t, logrus.DebugLevel, logrus.GetLevel())
}
