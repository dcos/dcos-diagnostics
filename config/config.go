package config

var (
	// Version of dcos-diagnostics code.
	Version = "0.4.0"

	// APIVer is an API version.
	APIVer = 1
)

// Config structure is a main config object
type Config struct {
	SystemdUnits []string

	// dcos-diagnostics flags
	FlagCACertFile                 string `mapstructure:"ca-cert"`
	FlagPull                       bool   `mapstructure:"pull"`
	FlagVerbose                    bool   `mapstructure:"verbose"`
	FlagPort                       int    `mapstructure:"port"`
	FlagDisableUnixSocket          bool   `mapstructure:"no-unix-socket"`
	FlagMasterPort                 int    `mapstructure:"master-port"`
	FlagAgentPort                  int    `mapstructure:"agent-port"`
	FlagPullInterval               int    `mapstructure:"pull-interval"`
	FlagPullTimeoutSec             int    `mapstructure:"pull-timeout"`
	FlagUpdateHealthReportInterval int    `mapstructure:"health-update-interval"`
	FlagExhibitorClusterStatusURL  string `mapstructure:"exhibitor-ip"`
	FlagForceTLS                   bool   `mapstructure:"force-tls"`
	FlagDebug                      bool   `mapstructure:"debug"`
	FlagRole                       string `mapstructure:"role"`
	FlagIAMConfig                  string `mapstructure:"iam-config"`

	// diagnostics job flags
	FlagDiagnosticsBundleDir                     string `mapstructure:"diagnostics-bundle-dir"`
	FlagDiagnosticsBundleEndpointsConfigFile     string `mapstructure:"endpoint-config"`
	FlagDiagnosticsBundleUnitsLogsSinceString    string `mapstructure:"diagnostics-units-since"`
	FlagDiagnosticsJobTimeoutMinutes             int    `mapstructure:"diagnostics-job-timeout"`
	FlagDiagnosticsJobGetSingleURLTimeoutMinutes int    `mapstructure:"diagnostics-url-timeout"`
	FlagCommandExecTimeoutSec                    int    `mapstructure:"command-exec-timeout"`
}
