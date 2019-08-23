package rest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dcos/dcos-diagnostics/collector"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// work dir contains only directories, each dir is created for single bundle (id is its name) and should contain:
const (
	stateFileName = "state.json" // file with information about diagnostics run
	dataFileName  = "file.zip"   // data gathered by diagnostics

	summaryErrorsReportFileName = "summaryErrorsReport.txt" // error log in bundle
	summaryReportFileName       = "summaryReport.txt"       // error log in bundle

	filePerm = 0600
	dirPerm  = 0700
)

type Bundle struct {
	ID      string    `json:"id,omitempty"`
	Type    Type      `json:"type"`
	Size    int64     `json:"size,omitempty"` // length in bytes for regular files; 0 when Canceled or Deleted
	Status  Status    `json:"status"`
	Started time.Time `json:"started_at,omitempty"`
	Stopped time.Time `json:"stopped_at,omitempty"`
	Errors  []string  `json:"errors,omitempty"`
}

type ErrorResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func NewBundleHandler(workDir string, collectors []collector.Collector, timeout time.Duration) (*BundleHandler, error) {
	err := initializeWorkDir(workDir)
	if err != nil {
		return nil, err
	}

	return &BundleHandler{
		stateFileLock:         &sync.RWMutex{},
		clock:                 realClock{},
		workDir:               workDir,
		collectors:            collectors,
		bundleCreationTimeout: timeout,
	}, nil
}

// BundleHandler is a struct that collects all functions
// responsible for diagnostics bundle lifecycle
type BundleHandler struct {
	stateFileLock         *sync.RWMutex // used to synchronize access to state file
	clock                 Clock
	workDir               string                // location where bundles are generated and stored
	collectors            []collector.Collector // information what should be in the bundle
	bundleCreationTimeout time.Duration         // limits how long bundle creation could take
}

type node struct {
	IP      net.IP `json:"ip"`
	Role    string `json:"role"`
	baseURL string
}

func (h BundleHandler) Create(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if h.bundleExists(id) {
		writeJSONError(w, http.StatusConflict, fmt.Errorf("bundle %s already exists", id))
		return
	}

	bundleWorkDir := filepath.Join(h.workDir, id)
	err := os.MkdirAll(bundleWorkDir, dirPerm)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create bundle %s workdir: %s", id, err))
		return
	}

	bundle := Bundle{
		ID:      id,
		Started: h.clock.Now(),
		Status:  Started,
	}

	bundleStatus, err := h.writeStateFile(bundle)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not update state file %s: %s", id, err))
		return
	}

	dataFile, err := os.Create(filepath.Join(h.workDir, id, dataFileName))
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not create data file %s: %s", id, err))
	}

	//TODO(janisz): use context cancel function to cancel bundle creation https://jira.mesosphere.com/browse/DCOS_OSS-5222
	ctx, _ := context.WithTimeout(context.Background(), h.bundleCreationTimeout) //nolint:govet
	done := make(chan []string)

	go collectAll(ctx, done, dataFile, h.collectors)

	go func() {
		select {
		case <-ctx.Done():
			break
		case bundle.Errors = <-done:
			bundle.Status = Done
			bundle.Stopped = h.clock.Now()
			if _, e := h.writeStateFile(bundle); e != nil {
				logrus.WithError(e).Errorf("Could not update state file %s", id)
			}
		}
	}()

	write(w, bundleStatus)
}

func collectAll(ctx context.Context, done chan<- []string, dataFile io.WriteCloser, collectors []collector.Collector) {
	zipWriter := zip.NewWriter(dataFile)
	var errors []string
	// summaryReport is a log of a diagnostics job
	summaryReport := new(bytes.Buffer)

	for _, c := range collectors {
		if ctx.Err() != nil {
			errors = append(errors, ctx.Err().Error())
			break
		}
		summaryReport.WriteString(fmt.Sprintf("[START GET %s]\n", c.Name()))
		err := collect(ctx, c, zipWriter)
		summaryReport.WriteString(fmt.Sprintf("[STOP GET %s]\n", c.Name()))
		if err != nil && !c.Optional() {
			errors = append(errors, err.Error())
		}
	}

	summaryReportFile, err := zipWriter.Create(summaryReportFileName)
	if err != nil {
		errors = append(errors, err.Error())
	} else {
		if _, err := io.Copy(summaryReportFile, summaryReport); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) != 0 {
		summaryErrorReportFile, err := zipWriter.Create(summaryErrorsReportFileName)
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			if _, err := summaryErrorReportFile.Write([]byte(strings.Join(errors, "\n"))); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if err := zipWriter.Close(); err != nil {
		errors = append(errors, err.Error())
	}
	if err := dataFile.Close(); err != nil {
		errors = append(errors, err.Error())
	}

	done <- errors
}

func collect(ctx context.Context, c collector.Collector, zipWriter *zip.Writer) error {
	rc, err := c.Collect(ctx)
	if err != nil {
		if !c.Optional() {
			return fmt.Errorf("could not collect %s: %s", c.Name(), err)
		}
		rc = ioutil.NopCloser(bytes.NewReader([]byte(err.Error())))
	}
	defer rc.Close()

	zipFile, err := zipWriter.Create(c.Name())
	if err != nil {
		return fmt.Errorf("could not create a %s in the zip: %s", c.Name(), err)
	}
	if _, err := io.Copy(zipFile, rc); err != nil {
		return fmt.Errorf("could not copy data to zip: %s", err)
	}

	return nil
}

func (h BundleHandler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if !h.bundleExists(id) {
		http.NotFound(w, r)
		return
	}

	bundle, err := h.getBundleState(id)
	if err != nil {
		bundle.Errors = append(bundle.Errors, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		logrus.WithField("ID", id).WithError(err).Warn("There is a problem with the bundle")
	}

	write(w, jsonMarshal(bundle))
}

func (h BundleHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	bundle, err := h.getBundleState(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if bundle.Status == Deleted || bundle.Status == Canceled || bundle.Status == Failed {
		writeJSONError(w, http.StatusNotFound,
			fmt.Errorf("bundle %s was %s", bundle.ID, bundle.Status))
		return
	}

	if bundle.Status != Done {
		writeJSONError(w, http.StatusNotFound,
			fmt.Errorf("bundle %s is not done yet (status %s), try again later", bundle.ID, bundle.Status))
		return
	}

	w.Header().Add("Content-Type", "application/zip, application/octet-stream")
	w.Header().Add("Content-disposition", fmt.Sprintf("attachment; filename=%s.zip", id))
	http.ServeFile(w, r, filepath.Join(h.workDir, id, dataFileName))
}

func (h BundleHandler) List(w http.ResponseWriter, r *http.Request) {
	ids, err := ioutil.ReadDir(h.workDir)
	if err != nil {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Errorf("could not read work dir: %s", err))
		return
	}

	bundles := make([]Bundle, 0, len(ids))

	for _, id := range ids {
		if !id.IsDir() {
			continue
		}

		bundle, err := h.getBundleState(id.Name())
		if err != nil {
			logrus.WithField("ID", id.Name()).WithError(err).Warn("There is a problem with the bundle")
		}
		bundles = append(bundles, bundle)

	}

	write(w, jsonMarshal(bundles))
}

func (h BundleHandler) getBundleState(id string) (Bundle, error) {
	bundle := Bundle{
		ID:     id,
		Status: Unknown,
	}

	rawState, err := h.readStateFile(bundle)
	if err != nil {
		return bundle, fmt.Errorf("could not read state file for bundle %s: %s", id, err)
	}

	err = json.Unmarshal(rawState, &bundle)
	if err != nil {
		return bundle, fmt.Errorf("could not unmarshal state file %s: %s", id, err)
	}

	if bundle.Status == Deleted || bundle.Status == Canceled || bundle.Status == Unknown {
		return bundle, nil
	}

	dataFileStat, err := os.Stat(filepath.Join(h.workDir, id, dataFileName))
	if err != nil {
		bundle.Status = Unknown
		return bundle, fmt.Errorf("could not stat data file %s: %s", id, err)
	}
	bundle.Size = dataFileStat.Size()

	return bundle, nil
}

func (h BundleHandler) bundleExists(id string) bool {
	s, err := os.Stat(filepath.Join(h.workDir, id))
	if os.IsNotExist(err) {
		return false
	}
	if !s.IsDir() {
		// If this is a file then it's not a valid bundle
		return false
	}
	return true
}

func (h BundleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if !h.bundleExists(id) {
		http.NotFound(w, r)
		return
	}

	bundle, err := h.getBundleState(id)
	if err != nil {
		logrus.WithField("ID", id).WithError(err).Warn("There is a problem with the bundle")
		bundle.Errors = append(bundle.Errors, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		write(w, jsonMarshal(bundle))
		return
	}

	if bundle.Status == Deleted || bundle.Status == Canceled {
		w.WriteHeader(http.StatusNotModified)
		write(w, jsonMarshal(bundle))
		return
	}

	//TODO(janisz): Handle Canceled Status https://jira.mesosphere.com/browse/DCOS_OSS-5222

	err = os.Remove(filepath.Join(h.workDir, id, dataFileName))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("could not delete bundle %s: %s", id, err))
		return
	}

	bundle.Status = Deleted
	newRawState, err := h.writeStateFile(bundle)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError,
			fmt.Errorf("bundle %s was deleted but state could not be updated: %s", id, err))
		return
	}
	write(w, newRawState)
}

func (h BundleHandler) writeStateFile(bundle Bundle) ([]byte, error) {
	stateFilePath := filepath.Join(h.workDir, bundle.ID, stateFileName)
	newRawState := jsonMarshal(bundle)
	h.stateFileLock.Lock()
	err := ioutil.WriteFile(stateFilePath, newRawState, filePerm)
	h.stateFileLock.Unlock()
	return newRawState, err
}

func (h BundleHandler) readStateFile(bundle Bundle) ([]byte, error) {
	stateFilePath := filepath.Join(h.workDir, bundle.ID, stateFileName)
	h.stateFileLock.RLock()
	defer h.stateFileLock.RUnlock()
	return ioutil.ReadFile(stateFilePath)
}

func writeJSONError(w http.ResponseWriter, code int, e error) {
	resp := ErrorResponse{Code: code, Error: e.Error()}
	body := jsonMarshal(resp)

	if e != nil {
		logrus.WithError(e).Errorf("Could not parse response: %s", e)
	}

	w.WriteHeader(code)
	write(w, body)
}

func write(w io.Writer, body []byte) {
	_, err := w.Write(body)
	if err != nil {
		logrus.WithError(err).Errorf("Could not write response")
	}
}

// jsonMarshal is a replacement for json.Marshal when we are 100% sure
// there won't now be any error on marshaling.
func jsonMarshal(v interface{}) []byte {
	rawJSON, err := json.Marshal(v)

	if err != nil {
		logrus.WithError(err).Fatalf("Could not marshal %v: %s", v, err)
	}
	return rawJSON
}

// initializeWorkDir will create the specified bundle working directory if it doesn't already exist
// and will do nothing if it does.
func initializeWorkDir(workDir string) error {
	err := os.MkdirAll(workDir, dirPerm)
	if err != nil {
		return fmt.Errorf("could not create workdir: %s", err)
	}
	return nil
}
