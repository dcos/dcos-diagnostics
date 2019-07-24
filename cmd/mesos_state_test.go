package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getMesosState(t *testing.T) {
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, r.URL.String(), "http://leader.mesos:5050/state")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	state, err := getMesosState(tr)

	assert.NoError(t, err)
	assert.Equal(t, "{}", state)
}

func Test_getMesosState_status_not_200(t *testing.T) {
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, r.URL.String(), "http://leader.mesos:5050/state")
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       ioutil.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	state, err := getMesosState(tr)

	assert.EqualError(t, err, "unexpected status code: 404")
	assert.Equal(t, "{}", state)
}

func Test_getMesosState_errored(t *testing.T) {
	tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, r.URL.String(), "http://leader.mesos:5050/state")
		return nil, fmt.Errorf("error")
	})

	state, err := getMesosState(tr)

	assert.EqualError(t, err, "Get http://leader.mesos:5050/state: error")
	assert.Empty(t, state)
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}
