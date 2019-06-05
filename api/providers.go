package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/dcos/dcos-diagnostics/collector"
	"github.com/dcos/dcos-diagnostics/util"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
)

// LogProviders a structure defines a list of Providers
type LogProviders struct {
	HTTPEndpoints []HTTPProvider
	LocalFiles    []FileProvider
	LocalCommands []CommandProvider
}

// HTTPProvider is a provider for fetching an HTTP endpoint.
type HTTPProvider struct {
	Port     int
	URI      string
	FileName string
	Role     []string
	Optional bool
}

// FileProvider is a local file provider.
type FileProvider struct {
	Location string
	Role     []string
	Optional bool
}

// CommandProvider is a local command to execute.
type CommandProvider struct {
	Command  []string
	Role     []string
	Optional bool
}

func loadProviders(cfg *config.Config, DCOSTools dcos.Tooler) (*LogProviders, error) {
	// load the internal providers
	internalProviders, err := loadInternalProviders(cfg, DCOSTools)
	if err != nil {
		return nil, fmt.Errorf("could not initialize internal log providers: %s", err)
	}
	// load the external providers from a cfg file
	externalProviders, err := loadExternalProviders(cfg.FlagDiagnosticsBundleEndpointsConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("could not initialize external log providers: %s", err)
	}

	return &LogProviders{
		HTTPEndpoints: append(internalProviders.HTTPEndpoints, externalProviders.HTTPEndpoints...),
		LocalFiles:    append(internalProviders.LocalFiles, externalProviders.LocalFiles...),
		LocalCommands: append(internalProviders.LocalCommands, externalProviders.LocalCommands...),
	}, nil
}

func loadExternalProviders(endpointsConfgFiles []string) (externalProviders LogProviders, err error) {
	for _, endpointsConfigFile := range endpointsConfgFiles {
		endpointsConfig, err := ioutil.ReadFile(endpointsConfigFile)
		if err != nil {
			return externalProviders, fmt.Errorf("could not read %s: %s", endpointsConfigFile, err)
		}
		var logProviders LogProviders
		if err = json.Unmarshal(endpointsConfig, &logProviders); err != nil {
			return externalProviders, fmt.Errorf("could not parse %s: %s", endpointsConfigFile, err)
		}
		externalProviders.HTTPEndpoints = append(externalProviders.HTTPEndpoints, logProviders.HTTPEndpoints...)
		externalProviders.LocalFiles = append(externalProviders.LocalFiles, logProviders.LocalFiles...)
		externalProviders.LocalCommands = append(externalProviders.LocalCommands, logProviders.LocalCommands...)
	}

	return externalProviders, nil
}

func loadSystemdCollectors(cfg *config.Config, DCOSTools dcos.Tooler) ([]collector.Collector, error) {
	units, err := DCOSTools.GetUnitNames()
	if err != nil {
		return nil, fmt.Errorf("could not get unit names: %s", err)
	}

	units = append(units, cfg.SystemdUnits...)
	collectors := make([]collector.Collector, 0, len(units))

	duration, err := time.ParseDuration(cfg.FlagDiagnosticsBundleUnitsLogsSinceString)
	if err != nil {
		return nil, fmt.Errorf("error parsing '%s': %s", cfg.FlagDiagnosticsBundleUnitsLogsSinceString, err)
	}

	for _, unit := range units {
		collectors = append(
			collectors,
			collector.NewSystemd(unit, false, unit, duration),
		)
	}

	return collectors, nil
}

func loadInternalProviders(cfg *config.Config, DCOSTools dcos.Tooler) (internalConfigProviders LogProviders, err error) {
	units, err := DCOSTools.GetUnitNames()
	if err != nil {
		return internalConfigProviders, err
	}

	role, err := DCOSTools.GetNodeRole()
	if err != nil {
		return internalConfigProviders, err
	}

	port, err := getPullPortByRole(cfg, role)
	if err != nil {
		return internalConfigProviders, err
	}

	// load default HTTP
	var httpEndpoints []HTTPProvider
	for _, unit := range append(units, cfg.SystemdUnits...) {
		httpEndpoints = append(httpEndpoints, HTTPProvider{
			Port:     port,
			URI:      fmt.Sprintf("%s/logs/units/%s", baseRoute, unit),
			FileName: unit,
		})
	}

	// add dcos-diagnostics health report.
	httpEndpoints = append(httpEndpoints, HTTPProvider{
		Port:     port,
		URI:      baseRoute,
		FileName: "dcos-diagnostics-health.json",
	})

	return LogProviders{
		HTTPEndpoints: httpEndpoints,
	}, nil
}

func LoadCollectors(cfg *config.Config, tools dcos.Tooler, client *http.Client) ([]collector.Collector, error) {

	collectors, err := loadSystemdCollectors(cfg, tools)
	if err != nil {
		return nil, fmt.Errorf("could load systemd collectors: %s", err)
	}

	// load the external providers from a cfg file
	providers, err := loadExternalProviders(cfg.FlagDiagnosticsBundleEndpointsConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("could not initialize external log providers: %s", err)
	}

	role, err := tools.GetNodeRole()
	if err != nil {
		return nil, fmt.Errorf("could not get role: %s", err)
	}

	port, err := getPullPortByRole(cfg, role)
	if err != nil {
		return nil, err
	}

	// add dcos-diagnostics health report.
	providers.HTTPEndpoints = append(providers.HTTPEndpoints, HTTPProvider{
		Port:     port,
		URI:      baseRoute,
		FileName: "dcos-diagnostics-health.json",
	})

	// set filename if not set, some endpoints might be named e.g., after corresponding unit
	for _, endpoint := range providers.HTTPEndpoints {
		if !roleMatched(role, endpoint.Role) {
			continue
		}

		fileName := fmt.Sprintf("%d-%s.json", endpoint.Port, util.SanitizeString(endpoint.URI))
		if endpoint.FileName != "" {
			fileName = endpoint.FileName
		}

		url, err := util.UseTLSScheme(fmt.Sprintf("http://%s:%d%s", cfg.FlagHostname, endpoint.Port, endpoint.URI), cfg.FlagForceTLS)
		if err != nil {
			return nil, fmt.Errorf("could not initialize internal log providers: %s", err)

		}

		c := collector.NewEndpoint(fileName, endpoint.Optional, url, client)
		collectors = append(collectors, c)
	}

	// trim left "/" and replace all slashes with underscores.
	for _, fileProvider := range providers.LocalFiles {
		if !roleMatched(role, fileProvider.Role) {
			continue
		}

		key := strings.Replace(strings.TrimLeft(fileProvider.Location, "/"), "/", "_", -1)
		c := collector.NewFile(key, fileProvider.Optional, fileProvider.Location)
		collectors = append(collectors, c)
	}

	// sanitize command to use as filename
	for _, commandProvider := range providers.LocalCommands {
		if !roleMatched(role, commandProvider.Role) {
			continue
		}

		cmdWithArgs := strings.Join(commandProvider.Command, "_")
		trimmedCmdWithArgs := strings.Replace(cmdWithArgs, "/", "", -1)
		key := fmt.Sprintf("%s.output", trimmedCmdWithArgs)
		c := collector.NewCmd(key, commandProvider.Optional, commandProvider.Command)
		collectors = append(collectors, c)

	}

	return collectors, nil
}
