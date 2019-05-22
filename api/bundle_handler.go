package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// work dir contains only directories, each dir is created for single bundle (id is its name) and should contains:
const (
	stateFileName = "state.json" // file with information about diagnostics run
	dataFileName  = "file.zip"   // data gathered by diagnostics
)

type Bundle struct {
	ID      string    `json:"id,omitempty"`
	Size    int64     `json:"size,omitempty"` // length in bytes for regular files; 0 when Canceled or Deleted
	Status  Status    `json:"status"`
	Started time.Time `json:"started_at,omitempty"`
	Stopped time.Time `json:"stopped_at,omitempty"`
}

type ErrorResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// bundleHandler is a struct that collects all functions
// responsible for diagnostics bundle lifecycle
type bundleHandler struct {
	workDir string // location where bundles are generated and stored
}

func (h bundleHandler) create(w http.ResponseWriter, r *http.Request) {

}

func (h bundleHandler) get(w http.ResponseWriter, r *http.Request) {

}

func (h bundleHandler) getFile(w http.ResponseWriter, r *http.Request) {

}

func (h bundleHandler) list(w http.ResponseWriter, r *http.Request) {
	ids, err := ioutil.ReadDir(h.workDir)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not read work dir: %s", err))
	}

	bundles := make([]*Bundle, 0, len(ids))

	for _, id := range ids {
		if !id.IsDir() {
			continue
		}

		bundle := Bundle{
			ID:     id.Name(),
			Status: Unknown,
		}

		bundles = append(bundles, &bundle)

		stateFilePath := filepath.Join(h.workDir, id.Name(), stateFileName)
		rawState, err := ioutil.ReadFile(stateFilePath)
		if err != nil {
			continue
		}

		err = json.Unmarshal(rawState, &bundle)
		if err != nil {
			logrus.WithError(err).WithField("ID", bundle.ID).Errorf("Could not unmarshal state file")
			continue
		}

		if bundle.Status == Deleted || bundle.Status == Canceled || bundle.Status == Unknown {
			continue
		}

		dataFileStat, err := os.Stat(filepath.Join(h.workDir, id.Name(), dataFileName))
		if err != nil {
			bundle.Status = Unknown
			continue
		}

		if bundle.Size != dataFileStat.Size() {
			bundle.Size = dataFileStat.Size()
			// Update status files
			bundleStatus, err := json.Marshal(bundle)
			if err != nil {
				continue
			}
			err = ioutil.WriteFile(stateFilePath, bundleStatus, 0644)
			if err != nil {
				continue
			}
		}
	}

	body, err := json.Marshal(bundles)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("could not marshal response: %s", err))
		return
	}

	_, err = w.Write(body)
	if err != nil {
		logrus.WithError(err).Errorf("Could not write response: %s", err)
	}
}

func (h bundleHandler) delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["uuid"]
	stateFilePath := filepath.Join(h.workDir, id, stateFileName)
	rawState, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("could not find bundle %s: %s", id, err))
		return
	}

	bundle := Bundle{
		ID:     id,
		Status: Unknown,
	}
	err = json.Unmarshal(rawState, &bundle)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("could not find bundle %s: %s", id, err))
		return
	}

	if bundle.Status == Deleted || bundle.Status == Canceled {
		w.WriteHeader(http.StatusNotModified)
		_, err := w.Write(rawState)
		if err != nil {
			logrus.WithError(err).Errorf("Could not write response: %s", err)
		}
		return
	}

	err = os.Remove(filepath.Join(h.workDir, id, dataFileName))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("could not delete bundle %s: %s", id, err))
		return
	}

	bundle.Status = Deleted
	newRawState, err := json.Marshal(bundle)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError,
			fmt.Errorf("bundle %s was deleted but state could not be updated: %s", id, err))
		return
	}
	err = ioutil.WriteFile(stateFilePath, newRawState, 0644)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError,
			fmt.Errorf("bundle %s was deleted but state could not be updated: %s", id, err))
		return
	}
	_, err = w.Write(newRawState)
	if err != nil {
		logrus.WithError(err).Errorf("Could not write response: %s", err)
	}

}

func writeJSONError(w http.ResponseWriter, code int, e error) {
	resp := ErrorResponse{Code: code, Error: e.Error()}
	body, err := json.Marshal(resp)

	w.WriteHeader(code)

	if err != nil {
		logrus.WithError(err).Errorf("Could not parse response: %s", e)
	}

	_, err = w.Write(body)
	if err != nil {
		logrus.WithError(err).Errorf("Could not write response: %s", e)
	}
}
