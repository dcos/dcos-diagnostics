package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

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
}

// FileProvider is a local file provider.
type FileProvider struct {
	Location string
	Role     []string
}

// CommandProvider is a local command to execute.
type CommandProvider struct {
	Command []string
	Role    []string
}

func loadProviders(cfg *config.Config, DCOSTools dcos.Tooler) (*LogProviders, error) {
	// load the internal providers
	internalProviders, err := loadInternalProviders(cfg, DCOSTools)
	if err != nil {
		return nil, fmt.Errorf("could not initialize internal log providers: %s", err)
	}
	// load the external providers from a cfg file
	externalProviders, err := loadExternalProviders(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not initialize external log providers: %s", err)
	}

	return &LogProviders{
		HTTPEndpoints: append(internalProviders.HTTPEndpoints, externalProviders.HTTPEndpoints...),
		LocalFiles:    append(internalProviders.LocalFiles, externalProviders.LocalFiles...),
		LocalCommands: append(internalProviders.LocalCommands, externalProviders.LocalCommands...),
	}, nil
}

func loadExternalProviders(cfg *config.Config) (externalProviders LogProviders, err error) {
	endpointsConfig, err := ioutil.ReadFile(cfg.FlagDiagnosticsBundleEndpointsConfigFile)
	if err != nil {
		return externalProviders, err
	}
	if err = json.Unmarshal(endpointsConfig, &externalProviders); err != nil {
		return externalProviders, err
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
