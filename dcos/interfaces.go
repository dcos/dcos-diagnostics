package dcos

import (
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/dcos/dcos-go/dcos"
)

// Health is a type to indicates health of the unit.
type Health int

// DefaultStateURL use https scheme
var DefaultStateURL = url.URL{
	Scheme: "https",
	Host:   net.JoinHostPort(dcos.DNSRecordLeader, strconv.Itoa(dcos.PortMesosMaster)),
	Path:   "/state",
}

const (
	// Unhealthy indicates Unit is not healthy
	Unhealthy = 0
	// Healthy indicates Unit is healthy
	Healthy = 1
	// Unknown indicates Unit health could not be determined
	Unknown = 3
)

const (
	// MasterRole DC/OS role for a master.
	MasterRole = dcos.RoleMaster

	// AgentRole DC/OS role for an agent.
	AgentRole = dcos.RoleAgent

	// AgentPublicRole DC/OS role for a public agent.
	AgentPublicRole = dcos.RoleAgentPublic
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

// Tooler DC/OS specific tools interface.
type Tooler interface {
	// open dbus connection
	InitializeUnitControllerConnection() error

	// close dbus connection
	CloseUnitControllerConnection() error

	// function to get Connection.GetUnitProperties(pname)
	// returns a maps of properties https://github.com/coreos/go-systemd/blob/master/dbus/methods.go#L176
	GetUnitProperties(string) (map[string]interface{}, error)

	// A wrapper to /opt/mesosphere/bin/detect_ip script
	// should return empty string if script fails.
	DetectIP() (string, error)

	// get system's hostname
	GetHostname() (string, error)

	// Detect node role: master/agent
	GetNodeRole() (string, error)

	// Get DC/OS systemd units on a system
	GetUnitNames() ([]string, error)

	// Get journal output
	GetJournalOutput(string) (string, error)

	// Get mesos node id, first argument is a function to determine a role.
	GetMesosNodeID() (string, error)

	// Get makes HTTP GET request, return read arrays of bytes
	Get(string, time.Duration) ([]byte, int, error)

	// Post makes HTTP GET request, return read arrays of bytes
	Post(string, time.Duration) ([]byte, int, error)

	// LookupMaster will lookup a masters in DC/OS cluster.
	// Initial lookup will be done by making HTTP GET request to exhibitor.If GET request fails, the next lookup
	// will failover to history service for one minute, it this fails or no nodes found, masters will be looked up
	// in history service for last hour.
	GetMasterNodes() ([]Node, error)
	//
	//// GetAgentsFromMaster will lookup agents in DC/OS cluster.
	GetAgentNodes() ([]Node, error)

	// Get timestamp
	GetTimestamp() time.Time
}
