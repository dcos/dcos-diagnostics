package rest

import (
	"context"
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoordinator_CreatorShouldCreateAbundleAndReturnUpdateChan(t *testing.T) {

	numGoroutine := runtime.NumGoroutine()

	client := new(MockClient)

	c := Coordinator{
		client: client,
	}

	ctx := context.TODO()

	client.On("Create", ctx, "192.0.2.1", "id").Return(&Bundle{ID: "id", Status: Started}, nil)
	client.On("Create", ctx, "192.0.2.2", "id").Return(&Bundle{ID: "id", Status: Started}, nil)
	client.On("Create", ctx, "192.0.2.3", "id").Return(&Bundle{ID: "id", Status: Started}, nil)

	client.On("Status", ctx, "192.0.2.1", "id").Return(&Bundle{ID: "id", Status: Done}, nil)
	client.On("Status", ctx, "192.0.2.2", "id").Return(&Bundle{ID: "id", Status: Done}, nil)
	client.On("Status", ctx, "192.0.2.3", "id").Return(&Bundle{ID: "id", Status: Done}, nil)

	node1 := node{IP: net.ParseIP("192.0.2.1")}
	node2 := node{IP: net.ParseIP("192.0.2.2")}
	node3 := node{IP: net.ParseIP("192.0.2.3")}

	s := c.Create(context.TODO(), "id", []node{node1, node2, node3})

	var statuses []BundleStatus

	assert.Equal(t, numberOfWorkers, runtime.NumGoroutine()-numGoroutine)

	for i := 0; i < 6; i++ {
		statuses = append(statuses, <-s)
	}

	expected := []BundleStatus{
		{ID: "id", node: node1},
		{ID: "id", node: node1, done: true},
		{ID: "id", node: node2},
		{ID: "id", node: node2, done: true},
		{ID: "id", node: node3},
		{ID: "id", node: node3, done: true},
	}

	for _, s := range statuses {
		assert.Contains(t, expected, s)
	}
}
