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
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/dcos/dcos-diagnostics/api"
	"github.com/dcos/dcos-diagnostics/api/rest"
	diagDcos "github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"

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
	tr, err := initTransport()
	if err != nil {
		logrus.WithError(err).Fatal("Could not start")
	}

	nodeInfo, err := getNodeInfo(tr)
	if err != nil {
		logrus.Fatalf("Could not initialize nodeInfo: %s", err)
	}

	if defaultConfig.FlagDiagnosticsBundleFetchersCount < 1 {
		logrus.Fatal("workers-count must be greater than 0")
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

	client := util.NewHTTPClient(defaultConfig.GetSingleEntryTimeout(), tr)

	collectors, err := api.LoadCollectors(defaultConfig, DCOSTools, client)
	if err != nil {
		logrus.Fatalf("Could not init collectors properly: %s", err)
	}

	bundleTimeout := time.Minute * time.Duration(defaultConfig.FlagDiagnosticsJobTimeoutMinutes)
	bundleHandler, err := rest.NewBundleHandler(
		defaultConfig.FlagDiagnosticsBundleDir,
		collectors,
		bundleTimeout,
		defaultConfig.GetSingleEntryTimeout(),
	)
	if err != nil {
		logrus.WithError(err).Fatal("BundleHandler could not be created")
	}
	diagClient := rest.NewDiagnosticsClient(client)
	coord := rest.NewParallelCoordinator(diagClient, time.Minute, defaultConfig.FlagDiagnosticsBundleDir)
	urlBuilder := diagDcos.NewURLBuilder(defaultConfig.FlagAgentPort, defaultConfig.FlagMasterPort, defaultConfig.FlagForceTLS)
	clusterBundleHandler, err := rest.NewClusterBundleHandler(coord, diagClient, DCOSTools, defaultConfig.FlagDiagnosticsBundleDir,
		bundleTimeout, &urlBuilder)
	if err != nil {
		logrus.WithError(err).Fatal("ClusterBundleHandler could not be created")
	}

	// Inject dependencies used for running dcos-diagnostics.
	dt := &api.Dt{
		Cfg:                  defaultConfig,
		DtDCOSTools:          DCOSTools,
		DtDiagnosticsJob:     diagnosticsJob,
		BundleHandler:        *bundleHandler,
		ClusterBundleHandler: clusterBundleHandler,
		RunPullerChan:        make(chan bool),
		RunPullerDoneChan:    make(chan bool),
		SystemdUnits:         &api.SystemdUnits{},
		MR:                   &api.MonitoringResponse{},
	}

	// start diagnostic server and expose endpoints.
	logrus.Info("Start dcos-diagnostics")

	// start pulling every 60 seconds.
	if defaultConfig.FlagPull {
		go api.StartPullWithInterval(dt)
	}

	router := api.NewRouter(dt)
	srv := &http.Server{Handler: router}

	c := make(chan os.Signal, 1)
	signal.Notify(c)
	go func() {
		s := <-c
		logrus.WithField("Signal", s).Info("Shutdown the serwer...")
		srv.Shutdown(context.Background())
	}()

	if defaultConfig.FlagDisableUnixSocket {
		logrus.Infof("Exposing dcos-diagnostics API on 0.0.0.0:%d", defaultConfig.FlagPort)
		srv.Addr = fmt.Sprintf(":%d", defaultConfig.FlagPort)
		err := srv.ListenAndServe()
		logrus.WithError(err).Infof("Server exited")
		return
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
	err = srv.Serve(listeners[0])
	logrus.WithError(err).Infof("Server exited")
}

func getNodeInfo(tr http.RoundTripper) (nodeutil.NodeInfo, error) {
	var options []nodeutil.Option
	defaultStateURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(dcos.DNSRecordLeader, strconv.Itoa(dcos.PortMesosMaster)),
		Path:   "/state",
	}
	if defaultConfig.FlagForceTLS {
		options = append(options, nodeutil.OptionMesosStateURL(defaultStateURL.String()))
	}
	if defaultConfig.FlagIPDiscoveryCommandLocation != "" {
		options = append(options, nodeutil.OptionDetectIP(defaultConfig.FlagIPDiscoveryCommandLocation))
	}
	return nodeutil.NewNodeInfo(util.NewHTTPClient(defaultConfig.GetSingleEntryTimeout(), tr), defaultConfig.FlagRole, options...)
}

func initTransport() (http.RoundTripper, error) {
	var transportOptions []transport.OptionTransportFunc
	if defaultConfig.FlagCACertFile != "" {
		transportOptions = append(transportOptions, transport.OptionCaCertificatePath(defaultConfig.FlagCACertFile))
	}
	if defaultConfig.FlagIAMConfig != "" {
		transportOptions = append(transportOptions, transport.OptionIAMConfigPath(defaultConfig.FlagIAMConfig))
	}
	tr, err := transport.NewTransport(transportOptions...)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize HTTP transport: %s", err)
	}

	return tr, nil
}
