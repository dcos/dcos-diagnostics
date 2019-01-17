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

// pull represents process that fetches a complete health status response
type pull struct {
	cfg                *config.Config
	tools              dcos.Tooler
	runPullerChan      <-chan bool
	runPullerDoneChan  chan<- bool
	monitoringResponse *MonitoringResponse
}

// StartPullWithInterval will start to pull a DC/OS cluster health status
func StartPullWithInterval(dt *Dt) {
	// Start infinite loop
	p := pull{
		cfg:                dt.Cfg,
		tools:              dt.DtDCOSTools,
		runPullerChan:      dt.RunPullerChan,
		runPullerDoneChan:  dt.RunPullerDoneChan,
		monitoringResponse: dt.MR,
	}
	for {
		p.runPull()
	inner:
		select {
		case <-p.runPullerChan:
			logrus.Debug("Update cluster health request recevied")
			p.runPull()
			p.runPullerDoneChan <- true
			goto inner

		case <-time.After(time.Duration(dt.Cfg.FlagPullInterval) * time.Second):
			logrus.Debugf("Update cluster health after %d interval", p.cfg.FlagPullInterval)
		}

	}
}

func (p *pull) runPull() {
	clusterNodes, err := p.tools.GetMasterNodes()
	if err != nil {
		logrus.Errorf("Could not get master nodes: %s", err)
	}

	agentNodes, err := p.tools.GetAgentNodes()
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
		go p.pullHostStatus(node, respChan, &wg)
	}
	wg.Wait()

	// update collected units/nodes health statuses
	p.updateHealthStatus(respChan)
}

// function builds a map of all unique units with status
func (p *pull) updateHealthStatus(responses <-chan *httpResponse) {
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
			p.monitoringResponse.UpdateMonitoringResponse(&MonitoringResponse{
				Nodes:       nodes,
				Units:       units,
				UpdatedTime: time.Now(),
			})
			return
		}
	}
}

func (p *pull) pullHostStatus(host dcos.Node, respChan chan<- *httpResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	var response httpResponse

	markNodeHealthAsUnknown := func(statusCode int) {
		response.Status = statusCode
		host.Health = dcos.Unknown
		response.Node = host
		respChan <- &response
	}

	port, err := getPullPortByRole(p.cfg, host.Role)
	if err != nil {
		logrus.WithError(err).Errorf("Could not get a port by role %s", host.Role)
		markNodeHealthAsUnknown(http.StatusServiceUnavailable)
		return
	}

	baseURL := fmt.Sprintf("http://%s:%d%s", host.IP, port, baseRoute)

	// UnitsRoute available in router.go
	url, err := util.UseTLSScheme(baseURL, p.cfg.FlagForceTLS)
	if err != nil {
		logrus.WithError(err).Error("Could not read useTLSScheme")
		markNodeHealthAsUnknown(http.StatusServiceUnavailable)
		return
	}

	// Make a request to get node units status
	// use fake interface implementation for tests
	timeout := time.Duration(p.cfg.FlagPullTimeoutSec) * time.Second
	body, statusCode, err := p.tools.Get(url, timeout)
	if statusCode != http.StatusOK {
		logrus.WithField("URL", url).WithField("Body", string(body)).Errorf("Bad response code %d", statusCode)
		markNodeHealthAsUnknown(statusCode)
		return
	}
	if err != nil {
		logrus.WithError(err).WithField("URL", url).Error("Could not HTTP GET")
		markNodeHealthAsUnknown(statusCode)
		return
	}

	// Response should be strictly mapped to jsonBodyStruct, otherwise skip it
	var jsonBody UnitsHealthResponseJSONStruct
	if err := json.Unmarshal(body, &jsonBody); err != nil {
		logrus.WithError(err).WithField("URL", url).Errorf("Could not deserialize json response from %s", host.IP)
		markNodeHealthAsUnknown(statusCode)
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
			Timestamp:  p.tools.GetTimestamp(),
			PrettyName: propertiesMap.PrettyName,
		})
	}
	response.Node = host
	respChan <- &response

}

func getPullPortByRole(cfg *config.Config, role string) (int, error) {
	var port int
	if role != dcos.MasterRole && role != dcos.AgentRole && role != dcos.AgentPublicRole {
		return port, fmt.Errorf("incorrect role %s, must be: %s, %s or %s", role, dcos.MasterRole, dcos.AgentRole, dcos.AgentPublicRole)
	}
	port = cfg.FlagAgentPort
	if role == dcos.MasterRole {
		port = cfg.FlagMasterPort
	}
	return port, nil
}
