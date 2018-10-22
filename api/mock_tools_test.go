package api

import (
	"github.com/dcos/dcos-diagnostics/dcos"
	"github.com/stretchr/testify/mock"
	"time"
)

type MockedTools struct{
	mock.Mock
}

func (m *MockedTools) InitializeUnitControllerConnection() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockedTools) CloseUnitControllerConnection() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockedTools) GetUnitProperties(s string) (map[string]interface{}, error) {
	args := m.Called(s)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockedTools) DetectIP() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockedTools) GetHostname() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockedTools) GetNodeRole() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockedTools) GetUnitNames() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockedTools) GetJournalOutput(string) (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockedTools) GetMesosNodeID() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockedTools) Get(s string, t time.Duration) ([]byte, int, error) {
	args := m.Called(s, t)
	return args.Get(0).([]byte), args.Int(1), args.Error(2)
}

func (m *MockedTools) Post(s string, t time.Duration) ([]byte, int, error) {
	args := m.Called(s, t)
	return args.Get(0).([]byte), args.Int(1), args.Error(2)
}

func (m *MockedTools) GetMasterNodes() ([]dcos.Node, error) {
	args := m.Called()
	return args.Get(0).([]dcos.Node), args.Error(1)
}

func (m *MockedTools) GetAgentNodes() ([]dcos.Node, error) {
	args := m.Called()
	return args.Get(0).([]dcos.Node), args.Error(1)
}

func (m *MockedTools) GetTimestamp() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}
