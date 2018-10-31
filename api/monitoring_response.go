package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/sirupsen/logrus"
)

type notFoundError struct {
	what string
}

func (e notFoundError) Error() string {
	return fmt.Sprintf("%s not found", e.what)
}

// MonitoringResponse top level global variable to store the entire units/nodes status tree.
type MonitoringResponse struct {
	sync.RWMutex

	Units       map[string]dcos.Unit
	Nodes       map[string]dcos.Node
	UpdatedTime time.Time
}

// UpdateMonitoringResponse will update the status tree.
func (mr *MonitoringResponse) UpdateMonitoringResponse(r *MonitoringResponse) {
	mr.Lock()
	defer mr.Unlock()
	mr.Nodes = r.Nodes
	mr.Units = r.Units
	mr.UpdatedTime = r.UpdatedTime
}

// GetAllUnits returns all systemd units from status tree.
func (mr *MonitoringResponse) GetAllUnits() UnitsResponseJSONStruct {
	mr.Lock()
	defer mr.Unlock()
	return UnitsResponseJSONStruct{
		Array: func() []UnitResponseFieldsStruct {
			var r []UnitResponseFieldsStruct
			for _, unit := range mr.Units {
				r = append(r, UnitResponseFieldsStruct{
					unit.UnitName,
					unit.PrettyName,
					unit.Health,
					unit.Title,
				})
			}
			return r
		}(),
	}
}

// GetUnit gets a specific Unit from a status tree.
func (mr *MonitoringResponse) GetUnit(unitName string) (UnitResponseFieldsStruct, error) {
	mr.Lock()
	defer mr.Unlock()
	fmt.Printf("Trying to look for %s\n", unitName)
	if _, ok := mr.Units[unitName]; !ok {
		return UnitResponseFieldsStruct{}, notFoundError{unitName}
	}

	return UnitResponseFieldsStruct{
		mr.Units[unitName].UnitName,
		mr.Units[unitName].PrettyName,
		mr.Units[unitName].Health,
		mr.Units[unitName].Title,
	}, nil

}

// GetNodesForUnit get all hosts for a specific Unit available in status tree.
func (mr *MonitoringResponse) GetNodesForUnit(unitName string) (NodesResponseJSONStruct, error) {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.Units[unitName]; !ok {
		return NodesResponseJSONStruct{}, notFoundError{unitName}
	}
	return NodesResponseJSONStruct{
		Array: func() []*NodeResponseFieldsStruct {
			var r []*NodeResponseFieldsStruct
			for _, node := range mr.Units[unitName].Nodes {
				r = append(r, &NodeResponseFieldsStruct{
					node.IP,
					node.Health,
					node.Role,
				})
			}
			return r
		}(),
	}, nil
}

// GetSpecificNodeForUnit gets a specific notFoundError{nodeIP}e.
func (mr *MonitoringResponse) GetSpecificNodeForUnit(unitName, nodeIP string) (NodeResponseFieldsWithErrorStruct, error) {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.Units[unitName]; !ok {
		return NodeResponseFieldsWithErrorStruct{}, notFoundError{unitName}
	}

	for _, node := range mr.Units[unitName].Nodes {
		if node.IP == nodeIP {
			helpField := fmt.Sprintf("Node available at `dcos node ssh -mesos-id %s`. Try, `journalctl -xv` to diagnose further.", node.MesosID)
			return NodeResponseFieldsWithErrorStruct{
				node.IP,
				node.Health,
				node.Role,
				node.Output[unitName],
				helpField,
			}, nil
		}
	}
	return NodeResponseFieldsWithErrorStruct{}, notFoundError{nodeIP}
}

// GetNodes gets all available nodes in status tree.
func (mr *MonitoringResponse) GetNodes() NodesResponseJSONStruct {
	mr.Lock()
	defer mr.Unlock()
	return NodesResponseJSONStruct{
		Array: func() []*NodeResponseFieldsStruct {
			var nodes []*NodeResponseFieldsStruct
			for _, node := range mr.Nodes {
				nodes = append(nodes, &NodeResponseFieldsStruct{
					node.IP,
					node.Health,
					node.Role,
				})
			}
			return nodes
		}(),
	}
}

// GetMasterAgentNodes returns a list of master and agent nodes available in status tree.
func (mr *MonitoringResponse) GetMasterAgentNodes() ([]dcos.Node, []dcos.Node, error) {
	mr.Lock()
	defer mr.Unlock()

	var masterNodes, agentNodes []dcos.Node
	for _, node := range mr.Nodes {
		if node.Role == dcos.MasterRole {
			masterNodes = append(masterNodes, node)
			continue
		}

		if node.Role == dcos.AgentRole || node.Role == dcos.AgentPublicRole {
			agentNodes = append(agentNodes, node)
		}
	}

	if len(masterNodes) == 0 && len(agentNodes) == 0 {
		logrus.Warn("no nodes found in memory, perhaps dcos-diagnostics was started without -pull flag")
		return masterNodes, agentNodes, notFoundError{"any nodes"}
	}

	return masterNodes, agentNodes, nil
}

// GetNodeByID returns a node by IP address from a status tree.
func (mr *MonitoringResponse) GetNodeByID(nodeIP string) (NodeResponseFieldsStruct, error) {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.Nodes[nodeIP]; !ok {
		return NodeResponseFieldsStruct{}, notFoundError{nodeIP}
	}
	return NodeResponseFieldsStruct{
		mr.Nodes[nodeIP].IP,
		mr.Nodes[nodeIP].Health,
		mr.Nodes[nodeIP].Role,
	}, nil
}

// GetNodeUnitsID returns a Unit status for a given node from status tree.
func (mr *MonitoringResponse) GetNodeUnitsID(nodeIP string) (UnitsResponseJSONStruct, error) {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.Nodes[nodeIP]; !ok {
		return UnitsResponseJSONStruct{}, notFoundError{nodeIP}
	}
	return UnitsResponseJSONStruct{
		Array: func(nodeIp string) []UnitResponseFieldsStruct {
			var units []UnitResponseFieldsStruct
			for _, unit := range mr.Nodes[nodeIp].Units {
				units = append(units, UnitResponseFieldsStruct{
					unit.UnitName,
					unit.PrettyName,
					unit.Health,
					unit.Title,
				})
			}
			return units
		}(nodeIP),
	}, nil
}

// GetNodeUnitByNodeIDUnitID returns a Unit status by node IP address and Unit ID.
func (mr *MonitoringResponse) GetNodeUnitByNodeIDUnitID(nodeIP, unitID string) (HealthResponseValues, error) {
	mr.Lock()
	defer mr.Unlock()
	if _, ok := mr.Nodes[nodeIP]; !ok {
		return HealthResponseValues{}, notFoundError{nodeIP}
	}
	for _, unit := range mr.Nodes[nodeIP].Units {
		if unit.UnitName == unitID {
			helpField := fmt.Sprintf("Node available at `dcos node ssh -mesos-id %s`. Try, `journalctl -xv` to diagnose further.", mr.Nodes[nodeIP].MesosID)
			return HealthResponseValues{
				UnitID:     unit.UnitName,
				UnitHealth: unit.Health,
				UnitOutput: mr.Nodes[nodeIP].Output[unit.UnitName],
				UnitTitle:  unit.Title,
				Help:       helpField,
				PrettyName: unit.PrettyName,
			}, nil
		}
	}
	return HealthResponseValues{}, notFoundError{unitID}
}

// GetLastUpdatedTime returns timestamp of latest updated monitoring response.
func (mr *MonitoringResponse) GetLastUpdatedTime() string {
	mr.Lock()
	defer mr.Unlock()
	if mr.UpdatedTime.IsZero() {
		return ""
	}
	return mr.UpdatedTime.Format(time.ANSIC)
}
