package dcos

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_findMastersInExhibitor_Find(t *testing.T) {
	called := false
	getFn := func(url string, duration time.Duration) ([]byte, int, error) {
		if called {
			return []byte("[]"), 200, nil
		}
		called = true
		body := `[
			{"Hostname": "mesos-master-1", "IsLeader": true},
			{"Hostname": "mesos-master-2", "IsLeader": false}
		]`
		return []byte(body), 200, nil
	}

	finder := findMastersInExhibitor{getFn: getFn}

	nodes, err := finder.Find()

	assert.NoError(t, err)
	assert.Equal(t, []Node{
		{Role: MasterRole, IP: "mesos-master-1", Leader: true},
		{Role: MasterRole, IP: "mesos-master-2"},
	}, nodes)

	nodes, err = finder.Find()

	assert.EqualError(t, err, "master nodes not found in exhibitor")
	assert.Empty(t, nodes)
}

func Test_findMastersInExhibitor_FindWithError(t *testing.T) {
	getFn := func(url string, duration time.Duration) ([]byte, int, error) {
		return nil, 0, fmt.Errorf("some error")
	}

	finder := findMastersInExhibitor{getFn: getFn}

	nodes, err := finder.Find()

	assert.EqualError(t, err, "some error")
	assert.Empty(t, nodes)
}

func Test_findMastersInExhibitor_FindWithEmptyBodyAndErrorCode(t *testing.T) {
	getFn := func(url string, duration time.Duration) ([]byte, int, error) {
		return []byte("message"), 518, nil
	}

	finder := findMastersInExhibitor{getFn: getFn}

	nodes, err := finder.Find()

	assert.EqualError(t, err, "GET  failed, status code: 518, body: message")
	assert.Empty(t, nodes)
}

func Test_findMastersInExhibitor_FindWithEmptyBody(t *testing.T) {
	getFn := func(url string, duration time.Duration) ([]byte, int, error) {
		return nil, 200, nil
	}

	finder := findMastersInExhibitor{getFn: getFn}

	nodes, err := finder.Find()

	assert.EqualError(t, err, "unexpected end of JSON input")
	assert.Empty(t, nodes)
}
