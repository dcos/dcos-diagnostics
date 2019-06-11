package dcos

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentNoTLS(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, false)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, AgentRole)
	assert.NoError(t, err, "")
	assert.Equal(t, "http://127.0.0.1:8080", url)
}

func TestMasterNoTLS(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, false)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, MasterRole)
	assert.NoError(t, err, "")
	assert.Equal(t, "http://127.0.0.1:8081", url)
}

func TestAgentTLS(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, true)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, AgentRole)
	assert.NoError(t, err, "")
	assert.Equal(t, "https://127.0.0.1:8080", url)
}

func TestMasterTLS(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, true)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, MasterRole)
	assert.NoError(t, err, "")
	assert.Equal(t, "https://127.0.0.1:8081", url)
}

func TestPublicAgent(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, false)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, AgentPublicRole)
	assert.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8080", url)
}

func TestInvalidRole(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, false)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, "not_a_role")
	assert.Error(t, err)
	assert.Empty(t, url)
}
