package cmd

import (
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getNodeInfo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	node, err := getNodeInfo(nil)
	assert.Nil(t, node)
	assert.EqualError(t, err, "Role paramter is invalid or empty. Got ")

	defaultConfig.FlagRole = "master"
	node, err = getNodeInfo(nil)
	assert.NoError(t, err)

	ip, err := node.DetectIP()
	assert.Empty(t, ip)
	assert.EqualError(t, err, "stat /opt/mesosphere/bin/detect_ip: no such file or directory")

	tmpFile, err := ioutil.TempFile(os.TempDir(), "detect-ip.")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write([]byte(`echo 192.0.2.1`))
	require.NoError(t, err)

	oldValue := defaultConfig.FlagIPDiscoveryCommandLocation
	defaultConfig.FlagIPDiscoveryCommandLocation = tmpFile.Name()
	defer func() { defaultConfig.FlagIPDiscoveryCommandLocation = oldValue }()

	node, err = getNodeInfo(nil)
	assert.NoError(t, err)
	ip, err = node.DetectIP()
	assert.NoError(t, err)
	assert.Equal(t, "192.0.2.1", ip.String())
}
