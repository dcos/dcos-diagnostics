package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dcos/dcos-diagnostics/collectors"
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

func LoadCollectors(cfg *config.Config, tools dcos.Tooler, client *http.Client) ([]collectors.Collector, error) {
	providers, err := loadProviders(cfg, tools)
	if err != nil {
		return nil, fmt.Errorf("could not init bundle handler: %s", err)
	}
	var coll []collectors.Collector

	role, err := tools.GetNodeRole()
	if err != nil {
		return nil, fmt.Errorf("could not get role: %s", err)
	}

	// set filename if not set, some endpoints might be named e.g., after corresponding unit
	for _, endpoint := range providers.HTTPEndpoints {
		if !roleMatched(role, endpoint.Role) {
			continue
		}

		fileName := fmt.Sprintf("%d-%s.json", endpoint.Port, util.SanitizeString(endpoint.URI))
		if endpoint.FileName != "" {
			fileName = endpoint.FileName
		}
		//TODO(janisz): Handle socket-only endpoints e.g., dcos-diagnostics
		url, err := util.UseTLSScheme(fmt.Sprintf("http://%s:%d%s", cfg.FlagHostname, endpoint.Port, endpoint.URI), cfg.FlagForceTLS)
		if err != nil {
			return nil, fmt.Errorf("could not initialize internal log providers: %s", err)

		}

		c := collectors.NewEndpointCollector(fileName, endpoint.Optional, url, client)
		coll = append(coll, c)
	}

	// trim left "/" and replace all slashes with underscores.
	for _, fileProvider := range providers.LocalFiles {
		if !roleMatched(role, fileProvider.Role) {
			continue
		}

		key := strings.Replace(strings.TrimLeft(fileProvider.Location, "/"), "/", "_", -1)
		c := collectors.NewFileCollector(key, fileProvider.Optional, fileProvider.Location)
		coll = append(coll, c)
	}

	// sanitize command to use as filename
	for _, commandProvider := range providers.LocalCommands {
		if !roleMatched(role, commandProvider.Role) {
			continue
		}

		cmdWithArgs := strings.Join(commandProvider.Command, "_")
		trimmedCmdWithArgs := strings.Replace(cmdWithArgs, "/", "", -1)
		key := fmt.Sprintf("%s.output", trimmedCmdWithArgs)
		c := collectors.NewCmdCollector(key, commandProvider.Optional, commandProvider.Command)
		coll = append(coll, c)

	}

	//TODO(janisz): Setup systemd units

	return coll, nil
}
