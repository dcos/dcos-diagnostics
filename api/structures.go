package api

import (
	"sync"
	"time"

	"github.com/dcos/dcos-diagnostics/config"
)

// MonitoringResponse top level global variable to store the entire units/nodes status tree.
type MonitoringResponse struct {
	sync.RWMutex

	Units       map[string]Unit
	Nodes       map[string]Node
	UpdatedTime time.Time
}

// Health is a type to indicates health of the unit.
type Health int

const (
	// Unhealthy indicates Unit is not healthy
	Unhealthy = 0
	// Healthy indicates Unit is healthy
	Healthy = 1
)

// Unit for stands for systemd unit.
type Unit struct {
	UnitName   string
	Nodes      []Node `json:",omitempty"`
	Health     Health
	Title      string
	Timestamp  time.Time
	PrettyName string
}

// Node for DC/OS node.
type Node struct {
	Leader  bool
	Role    string
	IP      string
	Host    string
	Health  Health
	Output  map[string]string
	Units   []Unit `json:",omitempty"`
	MesosID string
}

// httpResponse a structure of http response from a remote host.
type httpResponse struct {
	Status int
	Units  []Unit
	Node   Node
}

// UnitsHealthResponseJSONStruct json response /system/health/v1
type UnitsHealthResponseJSONStruct struct {
	Array       []HealthResponseValues `json:"units"`
	Hostname    string                 `json:"hostname"`
	IPAddress   string                 `json:"ip"`
	DcosVersion string                 `json:"dcos_version"`
	Role        string                 `json:"node_role"`
	MesosID     string                 `json:"mesos_id"`
	TdtVersion  string                 `json:"dcos_diagnostics_version"`
}

// HealthResponseValues is a health values json response.
type HealthResponseValues struct {
	UnitID     string `json:"id"`
	UnitHealth Health `json:"health"`
	UnitOutput string `json:"output"`
	UnitTitle  string `json:"description"`
	Help       string `json:"help"`
	PrettyName string `json:"name"`
}

// UnitsResponseJSONStruct contains health overview, collected from all hosts
type UnitsResponseJSONStruct struct {
	Array []UnitResponseFieldsStruct `json:"units"`
}

// UnitResponseFieldsStruct contains systemd unit health report.
type UnitResponseFieldsStruct struct {
	UnitID     string `json:"id"`
	PrettyName string `json:"name"`
	UnitHealth Health `json:"health"`
	UnitTitle  string `json:"description"`
}

// NodesResponseJSONStruct contains an array of responses from nodes.
type NodesResponseJSONStruct struct {
	Array []*NodeResponseFieldsStruct `json:"nodes"`
}

// NodeResponseFieldsStruct contains a response from a node.
type NodeResponseFieldsStruct struct {
	HostIP     string `json:"host_ip"`
	NodeHealth Health `json:"health"`
	NodeRole   string `json:"role"`
}

// NodeResponseFieldsWithErrorStruct contains node response with errors.
type NodeResponseFieldsWithErrorStruct struct {
	HostIP     string `json:"host_ip"`
	NodeHealth Health `json:"health"`
	NodeRole   string `json:"role"`
	UnitOutput string `json:"output"`
	Help       string `json:"help"`
}

// Agent response json format
type agentsResponse struct {
	Agents []struct {
		Hostname   string `json:"hostname"`
		Attributes struct {
			PublicIP string `json:"public_ip"`
		} `json:"attributes"`
	} `json:"slaves"`
}

type exhibitorNodeResponse struct {
	Code        int
	Description string
	Hostname    string
	IsLeader    bool
}

// Dt is a struct of dependencies used in dcos-diagnostics code. There are 2 implementations, the one runs on a real system and
// the one used for testing.
type Dt struct {
	Cfg               *config.Config
	DtDCOSTools       DCOSHelper
	DtDiagnosticsJob  *DiagnosticsJob
	RunPullerChan     chan bool
	RunPullerDoneChan chan bool
	SystemdUnits      *SystemdUnits
	MR                *MonitoringResponse
}

type bundle struct {
	File string `json:"file_name"`
	Size int64  `json:"file_size"`
}

// UnitPropertiesResponse is a structure to unmarshal dbus.GetunitProperties response
type UnitPropertiesResponse struct {
	ID             string `json:"Id"`
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
