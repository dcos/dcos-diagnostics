package api

import (
"github.com/stretchr/testify/assert"
"path/filepath"
"testing"
)

func TestLoadSystemdCollectors(t *testing.T) {
	t.Parallel()
	tools := new(MockedTools)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := loadSystemdCollectors(cfg, tools)

	assert.Nil(t, err)
	assert.Empty(t, got)
}
