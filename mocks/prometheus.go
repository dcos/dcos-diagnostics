package mocks

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
)

type MockObserver struct {
	mock.Mock
}

func (m *MockObserver) Observe(v float64) {
	m.Called(v)
}

type MockHistogram struct {
	mock.Mock
}

func (m *MockHistogram) GetMetricWith(labels prometheus.Labels) (prometheus.Observer, error) {
	args := m.Called(labels)
	return args.Get(0).(prometheus.Observer), args.Error(1)
}

func (m *MockHistogram) GetMetricWithLabelValues(lvs ...string) (prometheus.Observer, error) {
	args := m.Called(lvs)
	return args.Get(0).(prometheus.Observer), args.Error(1)
}

func (m *MockHistogram) With(labels prometheus.Labels) prometheus.Observer {
	args := m.Called(labels)
	return args.Get(0).(prometheus.Observer)
}

func (m *MockHistogram) WithLabelValues(lvs ...string) prometheus.Observer {
	new := make([]interface{}, len(lvs))
	for i, v := range lvs {
		new[i] = v
	}
	args := m.Called(new...)
	return args.Get(0).(prometheus.Observer)
}

func (m *MockHistogram) CurryWith(labels prometheus.Labels) (prometheus.ObserverVec, error) {
	args := m.Called(labels)
	return args.Get(0).(prometheus.ObserverVec), args.Error(1)
}

func (m *MockHistogram) MustCurryWith(labels prometheus.Labels) prometheus.ObserverVec {
	args := m.Called(labels)
	return args.Get(0).(prometheus.ObserverVec)
}
func (m *MockHistogram) Describe(ch chan<- *prometheus.Desc) {
	m.Called(ch)
}

func (m *MockHistogram) Collect(ch chan<- prometheus.Metric) {
	m.Called(ch)
}
