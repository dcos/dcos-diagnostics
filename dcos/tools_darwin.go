package dcos

import (
	"errors"
	"net/http"
	"sync"

	"github.com/dcos/dcos-go/dcos/nodeutil"
)

// Tools is implementation of dcos.Tooler interface.
type Tools struct {
	sync.Mutex

	ExhibitorURL string
	Role         string
	ForceTLS     bool
	NodeInfo     nodeutil.NodeInfo
	Transport    http.RoundTripper

	hostname string
	ip       string
	mesosID  string
}

func (st *Tools) InitializeUnitControllerConnection() (err error) {
	return errors.New("does not work on darwin")
}

func (st *Tools) CloseUnitControllerConnection() error {
	return errors.New("does not work on darwin")
}

func (st *Tools) GetUnitProperties(pname string) (map[string]interface{}, error) {
	return nil, errors.New("does not work on darwin")
}

func (st *Tools) GetUnitNames() (units []string, err error) {
	return nil, errors.New("does not work on darwin")
}

func (st *Tools) GetJournalOutput(unit string) (string, error) {
	return "", errors.New("does not work on darwin")
}
