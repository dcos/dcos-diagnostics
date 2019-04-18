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
	"fmt"
	"net/http"

	"github.com/dcos/dcos-diagnostics/api"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-go/dcos/http/transport"
	"github.com/dcos/dcos-go/dcos/nodeutil"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	diagnosticsTCPPort        = 1050
	diagnosticsBundleDir      = "/var/run/dcos/dcos-diagnostics/diagnostic_bundles"
	diagnosticsEndpointConfig = "/opt/mesosphere/etc/endpoints_config.json"
	exhibitorURL              = "http://127.0.0.1:8181/exhibitor/v1/cluster/status"
)

// daemonCmd represents the daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start an http server and polls the components health.",
	Run: func(cmd *cobra.Command, args []string) {
		startDiagnosticsDaemon()
	},
}

func init() {
	daemonCmd.PersistentFlags().StringVar(&defaultConfig.FlagCACertFile, "ca-cert", defaultConfig.FlagCACertFile,
		"Use certificate authority.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagPort, "port", diagnosticsTCPPort,
		"Web server TCP port.")
	daemonCmd.PersistentFlags().BoolVar(&defaultConfig.FlagDisableUnixSocket, "no-unix-socket",
		defaultConfig.FlagDisableUnixSocket, "Disable use unix socket provided by systemd activation.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagMasterPort, "master-port", diagnosticsTCPPort,
		"Use TCP port to connect to masters.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagAgentPort, "agent-port", diagnosticsTCPPort,
		"Use TCP port to connect to agents.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagCommandExecTimeoutSec, "command-exec-timeout",
		120, "Set command executing timeout")
	daemonCmd.PersistentFlags().BoolVar(&defaultConfig.FlagPull, "pull", defaultConfig.FlagPull,
		"Try to pull runner from DC/OS hosts.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagPullInterval, "pull-interval", 60,
		"Set pull interval in seconds.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagPullTimeoutSec, "pull-timeout", 3,
		"Set pull timeout.")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagUpdateHealthReportInterval, "health-update-interval",
		60,
		"Set update health interval in seconds.")
	daemonCmd.PersistentFlags().StringVar(&defaultConfig.FlagExhibitorClusterStatusURL, "exhibitor-url", exhibitorURL,
		"Use Exhibitor URL to discover master nodes.")
	daemonCmd.PersistentFlags().BoolVar(&defaultConfig.FlagForceTLS, "force-tls", defaultConfig.FlagForceTLS,
		"Use HTTPS to do all requests.")
	daemonCmd.PersistentFlags().BoolVar(&defaultConfig.FlagDebug, "debug", defaultConfig.FlagDebug,
		"Enable pprof debugging endpoints.")
	daemonCmd.PersistentFlags().StringVar(&defaultConfig.FlagIAMConfig, "iam-config",
		defaultConfig.FlagIAMConfig, "A path to identity and access management config")
	// diagnostics job flags
	daemonCmd.PersistentFlags().StringVar(&defaultConfig.FlagDiagnosticsBundleDir,
		"diagnostics-bundle-dir", diagnosticsBundleDir, "Set a path to store diagnostic bundles")
	daemonCmd.PersistentFlags().StringSliceVar(&defaultConfig.FlagDiagnosticsBundleEndpointsConfigFiles,
		"endpoint-config", []string{diagnosticsEndpointConfig},
		"Use endpoints_config.json")
	daemonCmd.PersistentFlags().StringVar(&defaultConfig.FlagDiagnosticsBundleUnitsLogsSinceString,
		"diagnostics-units-since", "24h", "Collect systemd units logs since")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagDiagnosticsJobTimeoutMinutes,
		"diagnostics-job-timeout", 720,
		"Set a global diagnostics job timeout")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagDiagnosticsJobGetSingleURLTimeoutMinutes,
		"diagnostics-url-timeout", 2,
		"Set a local timeout for every single GET request to a log endpoint")
	daemonCmd.PersistentFlags().IntVar(&defaultConfig.FlagDiagnosticsBundleFetchersCount,
		"fetchers-count", 1,
		"Set a number of concurrent fetchers gathering nodes logs")
	RootCmd.AddCommand(daemonCmd)
}

func startDiagnosticsDaemon() {
	// init new transport
	var transportOptions []transport.OptionTransportFunc
	if defaultConfig.FlagCACertFile != "" {
		transportOptions = append(transportOptions, transport.OptionCaCertificatePath(defaultConfig.FlagCACertFile))
	}
	if defaultConfig.FlagIAMConfig != "" {
		transportOptions = append(transportOptions, transport.OptionIAMConfigPath(defaultConfig.FlagIAMConfig))
	}

	tr, err := transport.NewTransport(transportOptions...)
	if err != nil {
		logrus.Fatalf("Unable to initialize HTTP transport: %s", err)
	}

	client := &http.Client{
		Transport: tr,
	}

	var options []nodeutil.Option
	if defaultConfig.FlagForceTLS {
		options = append(options, nodeutil.OptionMesosStateURL(dcos.DefaultStateURL.String()))
	}

	nodeInfo, err := nodeutil.NewNodeInfo(client, defaultConfig.FlagRole, options...)
	if err != nil {
		logrus.Fatalf("Could not initialize nodeInfo: %s", err)
	}

	if defaultConfig.FlagDiagnosticsBundleFetchersCount < 1 {
		logrus.Fatalf("workers-count must be greater than 0")
	}

	DCOSTools := &dcos.Tools{
		ExhibitorURL: defaultConfig.FlagExhibitorClusterStatusURL,
		ForceTLS:     defaultConfig.FlagForceTLS,
		Role:         defaultConfig.FlagRole,
		NodeInfo:     nodeInfo,
		Transport:    tr,
	}

	// Create and init diagnostics job, do not hard fail on error
	diagnosticsJob := &api.DiagnosticsJob{
		Transport: tr,
		Cfg:       defaultConfig,
		DCOSTools: DCOSTools,
	}

	err = diagnosticsJob.Init()
	if err != nil {
		logrus.Fatalf("Could not init diagnostics job properly: %s", err)
	}

	// Inject dependencies used for running dcos-diagnostics.
	dt := &api.Dt{
		Cfg:               defaultConfig,
		DtDCOSTools:       DCOSTools,
		DtDiagnosticsJob:  diagnosticsJob,
		RunPullerChan:     make(chan bool),
		RunPullerDoneChan: make(chan bool),
		SystemdUnits:      &api.SystemdUnits{},
		MR:                &api.MonitoringResponse{},
	}

	// start diagnostic server and expose endpoints.
	logrus.Info("Start dcos-diagnostics")

	// start pulling every 60 seconds.
	if defaultConfig.FlagPull {
		go api.StartPullWithInterval(dt)
	}

	router := api.NewRouter(dt)

	if defaultConfig.FlagDisableUnixSocket {
		logrus.Infof("Exposing dcos-diagnostics API on 0.0.0.0:%d", defaultConfig.FlagPort)
		logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", defaultConfig.FlagPort), router))
	}

	// try using systemd socket
	// listeners, err := activation.Listeners(true)
	listeners, err := getListener(true)
	if err != nil {
		logrus.Fatalf("Unable to initialize listener: %s", err)
	}

	if len(listeners) == 0 || listeners[0] == nil {
		logrus.Fatal("Unix socket not found")
	}
	logrus.Infof("Using socket: %s", listeners[0].Addr().String())
	logrus.Fatal(http.Serve(listeners[0], router))
}
