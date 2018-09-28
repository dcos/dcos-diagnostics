package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dcos/dcos-diagnostics/config"
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/dcos/dcos-diagnostics/util"
	"github.com/sirupsen/logrus"
)

// StartPullWithInterval will start to pull a DC/OS cluster health status
func StartPullWithInterval(dt *Dt) {
	// Start infinite loop
	for {
		runPull(dt)
	inner:
		select {
		case <-dt.RunPullerChan:
			logrus.Debug("Update cluster health request recevied")
			runPull(dt)
			dt.RunPullerDoneChan <- true
			goto inner

		case <-time.After(time.Duration(dt.Cfg.FlagPullInterval) * time.Second):
			logrus.Debugf("Update cluster health after %d interval", dt.Cfg.FlagPullInterval)
		}

	}
}

func runPull(dt *Dt) {
	clusterNodes, err := dt.DtDCOSTools.GetMasterNodes()
	if err != nil {
		logrus.Errorf("Could not get master nodes: %s", err)
	}

	agentNodes, err := dt.DtDCOSTools.GetAgentNodes()
	if err != nil {
		logrus.Errorf("Could not get agent nodes: %s", err)
	}

	clusterNodes = append(clusterNodes, agentNodes...)

	// If not nodes found we should wait for a timeout between trying the next pull.
	if len(clusterNodes) == 0 {
		logrus.Error("Could not find master or agent nodes")
		return
	}

	respChan := make(chan *httpResponse, len(clusterNodes))

	// Pull data from each host
	var wg sync.WaitGroup
	for _, node := range clusterNodes {
		wg.Add(1)
		go pullHostStatus(node, respChan, dt, &wg)
	}
	wg.Wait()

	// update collected units/nodes health statuses
	updateHealthStatus(respChan, dt)
}

// function builds a map of all unique units with status
func updateHealthStatus(responses <-chan *httpResponse, dt *Dt) {
	var (
		units = make(map[string]dcos.Unit)
		nodes = make(map[string]dcos.Node)
	)

	for {
		select {
		case response := <-responses:
			node := response.Node
			node.Units = response.Units
			nodes[response.Node.IP] = node

			for _, currentUnit := range response.Units {
				u, ok := units[currentUnit.UnitName]
				if ok {
					u.Nodes = append(u.Nodes, currentUnit.Nodes...)
					if currentUnit.Health > u.Health {
						u.Health = currentUnit.Health
					}
					units[currentUnit.UnitName] = u
				} else {
					units[currentUnit.UnitName] = currentUnit
				}
			}
		default:
			dt.MR.UpdateMonitoringResponse(&MonitoringResponse{
				Nodes:       nodes,
				Units:       units,
				UpdatedTime: time.Now(),
			})
			return
		}
	}
}

func pullHostStatus(host dcos.Node, respChan chan<- *httpResponse, dt *Dt, wg *sync.WaitGroup) {
	defer wg.Done()
	var response httpResponse
	port, err := getPullPortByRole(dt.Cfg, host.Role)
	if err != nil {
		logrus.Errorf("Could not get a port by role %s: %s", host.Role, err)
		response.Status = http.StatusServiceUnavailable
		host.Health = 3
		response.Node = host
		respChan <- &response
		return
	}

	baseURL := fmt.Sprintf("http://%s:%d%s", host.IP, port, baseRoute)

	// UnitsRoute available in router.go
	url, err := util.UseTLSScheme(baseURL, dt.Cfg.FlagForceTLS)
	if err != nil {
		logrus.Errorf("Could not read UseTLSScheme: %s", err)
		response.Status = http.StatusServiceUnavailable
		host.Health = 3
		response.Node = host
		respChan <- &response
		return
	}

	// Make a request to get node units status
	// use fake interface implementation for tests
	timeout := time.Duration(dt.Cfg.FlagPullTimeoutSec) * time.Second
	body, statusCode, err := dt.DtDCOSTools.Get(url, timeout)
	if statusCode != http.StatusOK {
		logrus.Errorf("Bad response code %d. URL %s", statusCode, url)
		response.Status = statusCode
		host.Health = 3
		response.Node = host
		respChan <- &response
		return
	}
	if err != nil {
		logrus.Errorf("Could not HTTP GET %s: %s", url, err)
		response.Status = statusCode
		host.Health = 3 // 3 stands for unknown
		respChan <- &response
		response.Node = host
		return
	}

	// Response should be strictly mapped to jsonBodyStruct, otherwise skip it
	var jsonBody UnitsHealthResponseJSONStruct
	if err := json.Unmarshal(body, &jsonBody); err != nil {
		logrus.Errorf("Could not deserialize json response from %s, url %s: %s", host.IP, url, err)
		response.Status = statusCode
		host.Health = 3 // 3 stands for unknown
		respChan <- &response
		response.Node = host
		return
	}
	response.Status = statusCode

	// Update Response and send it back to respChan
	host.Host = jsonBody.Hostname

	// update mesos node id
	host.MesosID = jsonBody.MesosID

	host.Output = make(map[string]string)

	// if at least one Unit is not Healthy, the host should be set Unhealthy
	for _, propertiesMap := range jsonBody.Array {
		if propertiesMap.UnitHealth > host.Health {
			host.Health = propertiesMap.UnitHealth
			break
		}
	}

	for _, propertiesMap := range jsonBody.Array {
		// update error message per host per Unit
		host.Output[propertiesMap.UnitID] = propertiesMap.UnitOutput
		response.Units = append(response.Units, dcos.Unit{
			UnitName:   propertiesMap.UnitID,
			Nodes:      []dcos.Node{host},
			Health:     propertiesMap.UnitHealth,
			Title:      propertiesMap.UnitTitle,
			Timestamp:  dt.DtDCOSTools.GetTimestamp(),
			PrettyName: propertiesMap.PrettyName,
		})
	}
	response.Node = host
	respChan <- &response

}

func getPullPortByRole(cfg *config.Config, role string) (int, error) {
	var port int
	if role != dcos.MasterRole && role != dcos.AgentRole && role != dcos.AgentPublicRole {
		return port, fmt.Errorf("Incorrect role %s, must be: %s, %s or %s", role, dcos.MasterRole, dcos.AgentRole, dcos.AgentPublicRole)
	}
	port = cfg.FlagAgentPort
	if role == dcos.MasterRole {
		port = cfg.FlagMasterPort
	}
	return port, nil
}
