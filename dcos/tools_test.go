package dcos

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/jarcoal/httpmock.v1"
	"testing"
)

func TestDCOSTools_GetHostname(t *testing.T) {
	tools := &DCOSTools{
		hostname: "some hostname",
	}

	hostname, err := tools.GetHostname()

	assert.Equal(t, "some hostname", hostname)
	assert.NoError(t, err)

}

func TestDCOSTools_Get(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "https://some.url:8080",
		httpmock.NewStringResponder(200, `OK`))

	tools := &DCOSTools{
		Transport: httpmock.DefaultTransport,
	}

	body, code, err := tools.Get("https://some.url:8080", 0)

	assert.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Equal(t, "OK", string(body))

}

func TestDCOSTools_Post(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://some.url:8080",
		httpmock.NewStringResponder(200, `OK`))

	tools := &DCOSTools{
		Transport: httpmock.DefaultTransport,
	}

	body, code, err := tools.Post("https://some.url:8080", 0)

	assert.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Equal(t, "OK", string(body))
}

func TestDCOSTools_GetAgentNodes(t *testing.T) {
	dcosHistoryPath = "testdata/valid"

	tools := &DCOSTools{
		Transport: httpmock.DefaultTransport,
	}

	nodes, err := tools.GetAgentNodes()

	assert.NoError(t, err)
	assert.Equal(t, []Node{{Role: AgentRole, IP: "172.17.0.3"}}, nodes)

	nodes, err = tools.GetMasterNodes()
}

func TestDCOSTools_GetAgentNodes_WithInvalidData(t *testing.T) {
	dcosHistoryPath = "testdata/invalid"

	tools := &DCOSTools{
		Transport: httpmock.DefaultTransport,
	}

	nodes, err := tools.GetAgentNodes()

	assert.EqualError(t, err, "Agent nodes were not found in history service for the past /hour/")
	assert.Empty(t, nodes)
}

func TestDCOSTools_GetAgentNodes_WithoutHistoryDirectory(t *testing.T) {
	dcosHistoryPath = "not/existing/dir"

	tools := &DCOSTools{
		Transport: httpmock.DefaultTransport,
	}

	nodes, err := tools.GetAgentNodes()

	assert.EqualError(t, err, "open not/existing/dir/hour/: no such file or directory")
	assert.Empty(t, nodes)
}

