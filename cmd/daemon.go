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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dcos/dcos-diagnostics/api"
	"github.com/dcos/dcos-diagnostics/api/rest"
	"github.com/dcos/dcos-diagnostics/util"

	diagDcos "github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-go/dcos"
	"github.com/dcos/dcos-go/dcos/http/transport"
	"github.com/dcos/dcos-go/dcos/nodeutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	defaultStateURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(dcos.DNSRecordLeader, strconv.Itoa(dcos.PortMesosMaster)),
		Path:   "/state",
	}
	if defaultConfig.FlagForceTLS {
		options = append(options, nodeutil.OptionMesosStateURL(defaultStateURL.String()))
	}

	nodeInfo, err := nodeutil.NewNodeInfo(client, defaultConfig.FlagRole, options...)
	if err != nil {
		logrus.Fatalf("Could not initialize nodeInfo: %s", err)
	}

	if defaultConfig.FlagDiagnosticsBundleFetchersCount < 1 {
		logrus.Fatalf("workers-count must be greater than 0")
	}

	DCOSTools := &diagDcos.Tools{
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
		FetchPrometheusVector: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "fetch_endpoint_time_seconds",
			Help: "Time taken fetch single endpoint",
		}, []string{"path", "statusCode"}),
	}

	err = diagnosticsJob.Init()
	if err != nil {
		logrus.Fatalf("Could not init diagnostics job properly: %s", err)
	}

	urlTimeout := time.Minute * time.Duration(defaultConfig.FlagDiagnosticsJobGetSingleURLTimeoutMinutes)
	collectors, err := api.LoadCollectors(defaultConfig, DCOSTools, util.NewHTTPClient(urlTimeout, tr))
	if err != nil {
		logrus.Fatalf("Could not init collectors properly: %s", err)
	}

	bundleTimeout := time.Minute * time.Duration(defaultConfig.FlagDiagnosticsJobTimeoutMinutes)
	bundleHandler := rest.NewBundleHandler(defaultConfig.FlagDiagnosticsBundleDir, collectors, bundleTimeout)

	// Inject dependencies used for running dcos-diagnostics.
	dt := &api.Dt{
		Cfg:               defaultConfig,
		DtDCOSTools:       DCOSTools,
		DtDiagnosticsJob:  diagnosticsJob,
		BundleHanlder:     bundleHandler,
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
