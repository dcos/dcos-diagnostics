package dcos

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUrlBuilder(t *testing.T) {
	type test struct {
		role   string
		port   string
		useTLS bool
	}
	tests := map[string]test{
		"TestAgentNoTLS":       {role: AgentRole, port: "8080", useTLS: false},
		"TestAgentTLS":         {role: AgentRole, port: "8080", useTLS: true},
		"TestMasterNoTLS":      {role: MasterRole, port: "8081", useTLS: false},
		"TestMasterTLS":        {role: MasterRole, port: "8081", useTLS: true},
		"TestAgentPublicNoTLS": {role: AgentPublicRole, port: "8080", useTLS: false},
		"TestAgentPublicTLS":   {role: AgentPublicRole, port: "8080", useTLS: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := NewURLBuilder(8080, 8081, tc.useTLS)
			ip := net.IPv4(127, 0, 0, 1)
			url, err := builder.BaseURL(ip, tc.role)
			assert.NoError(t, err, "")
			if tc.useTLS {
				assert.Equal(t, "https://127.0.0.1:"+tc.port, url)
			} else {
				assert.Equal(t, "http://127.0.0.1:"+tc.port, url)
			}
		})
	}
}

func TestInvalidRole(t *testing.T) {
	builder := NewURLBuilder(8080, 8081, false)
	ip := net.IPv4(127, 0, 0, 1)

	url, err := builder.BaseURL(ip, "not_a_role")
	assert.Error(t, err)
	assert.Empty(t, url)
}
