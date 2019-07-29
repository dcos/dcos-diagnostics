package rest

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const numberOfWorkers = 10
const contextDoneErrMsg = "bundle creation context finished before bundle creation finished"

// BundleStatus tracks the status of local bundle creation requests
type BundleStatus struct {
	id   string
	node node
	done bool
	err  error
}

// golangcli-lint marks this as dead code because nothing uses the interface
// yet. this will be removed with the bundle handler PR
// coordinator is an interface to coordinate the creation of diagnostics bundles
// across a cluster of nodes
type Coordinator interface {
	// CreateBundle starts the bundle creation process. Status updates be monitored
	// on the returned channel.
	CreateBundle(ctx context.Context, id string, nodes []node) <-chan BundleStatus
	// CollectBundle waits until all the nodes' bundles have finished, downloads,
	// and merges them. The resulting bundle zip file path is returned.
	CollectBundle(ctx context.Context, bundleID string, numBundles int, statuses <-chan BundleStatus) (string, error)
}

// ParallelCoordinator implements Coordinator interface to coordinate bundle
// creation across a cluster, parallelized across multiple goroutines.
type ParallelCoordinator struct {
	client Client

	// statusCheckInterval defines how often the status of the local bundles will
	// be checked
	statusCheckInterval time.Duration
	workDir             string

	// quit is closed when the bundle is finished being created (whether due to a
	// canceled context or finishing fully). This is used to signal that the worker
	// goroutines should end, we use this rather than closing the jobs channel
	// because waitForDone will send jobs into the job channel so, if it's still
	// processing when the channel is closed, it would send to a closed channel,
	// causing a panic.
	quit chan struct{}
}

// NewParallelCoordinator creates and returns a new ParallelCoordinator
func NewParallelCoordinator(client Client, interval time.Duration, workDir string) *ParallelCoordinator {
	return &ParallelCoordinator{
		client:              client,
		statusCheckInterval: interval,
		workDir:             workDir,
		quit:                make(chan struct{}),
	}
}

// job is a function that will be called by the worker function. The output will be added to results channel
type job func(context.Context) BundleStatus

// worker is a function that will run incoming jobs from jobs channel and put jobs output to results chan
func worker(ctx context.Context, jobs <-chan job, results chan<- BundleStatus, quit <-chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case j := <-jobs:
			results <- j(ctx)
		}
	}
}

// CreateBundle starts the bundle creation process. Status updates be monitored
// on the returned channel.
func (c ParallelCoordinator) CreateBundle(ctx context.Context, id string, nodes []node) <-chan BundleStatus {

	jobs := make(chan job)
	statuses := make(chan BundleStatus)

	for i := 0; i < numberOfWorkers; i++ {
		go worker(ctx, jobs, statuses, c.quit)
	}

	for _, n := range nodes {
		logrus.WithField("IP", n.IP).Info("Sending creation request to node.")

		// necessary to prevent the closure from giving the same node to all the calls
		tmpNode := n
		jobs <- func(ctx context.Context) BundleStatus {
			select {
			case <-ctx.Done():
				return BundleStatus{
					id:   id,
					node: tmpNode,
					err:  errors.New(contextDoneErrMsg),
				}
			default:
			}

			return c.createBundle(ctx, tmpNode, id, jobs)
		}
	}

	return statuses
}

// CollectBundle waits until all the nodes' bundles have finished, downloads,
// and merges them. The resulting bundle zip file path is returned.
func (c ParallelCoordinator) CollectBundle(ctx context.Context, bundleID string, numBundles int, statuses <-chan BundleStatus) (string, error) {

	// holds the paths to the downloaded local bundles before merging
	var bundlePaths []string

	finishedBundles := 0
	for finishedBundles < numBundles {
		select {
		case <-ctx.Done():
			// TODO (https://jira.mesosphere.com/browse/DCOS_OSS-5303): this should be noted in the generated bundle and not just printed in the journal
			logrus.WithField("ID", bundleID).Warn("context cancelled before all node bundles finished")
			close(c.quit)
			return mergeZips(bundleID, bundlePaths, c.workDir)
		case s := <-statuses:
			if !s.done {
				logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.id).Info("Got status update. Bundle not ready.")
				continue
			}

			// even if the bundle finished with an error, it's now finished so increment finishedBundles
			finishedBundles++
			if s.err != nil {
				// TODO (https://jira.mesosphere.com/browse/DCOS_OSS-5303): this should be noted in the generated bundle and not just printed in the journal
				logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.id).Warn("Bundle errored")
				continue
			}

			bundlePath := filepath.Join(c.workDir, nodeBundleFilename(s.node))
			err := c.client.GetFile(ctx, s.node.baseURL, s.id, bundlePath)
			if err != nil {
				// TODO (https://jira.mesosphere.com/browse/DCOS_OSS-5303): this should be noted in the generated bundle and not just printed in the journal
				logrus.WithError(err).WithField("IP", s.node.IP).WithField("ID", s.id).Warn("Could not download file")
				continue
			}

			bundlePaths = append(bundlePaths, bundlePath)
		}
	}

	close(c.quit)
	return mergeZips(bundleID, bundlePaths, c.workDir)
}

func mergeZips(bundleID string, bundlePaths []string, workDir string) (string, error) {

	bundlePath := filepath.Join(workDir, fmt.Sprintf("bundle-%s.zip", bundleID))
	mergedZip, err := os.Create(bundlePath)
	if err != nil {
		return "", err
	}
	defer mergedZip.Close()

	zipWriter := zip.NewWriter(mergedZip)
	defer zipWriter.Close()

	for _, p := range bundlePaths {
		// TODO (https://jira.mesosphere.com/browse/DCOS_OSS-5303): this should be noted in the generated bundle
		err = appendToZip(zipWriter, p)
		if err != nil {
			return "", err
		}
	}

	return mergedZip.Name(), nil
}

func appendToZip(writer *zip.Writer, path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("could not open %s: %s", path, err)
	}
	defer r.Close()

	base := strings.TrimSuffix(filepath.Base(path), ".zip")

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("could not open %s from zip: %s", f.Name, err)
		}

		fileName := filepath.Join(base, f.Name)
		file, err := writer.Create(fileName)
		if err != nil {
			return fmt.Errorf("could not create file %s: %s", fileName, err)
		}
		_, err = io.Copy(file, rc)
		if err != nil {
			return fmt.Errorf("could not copy file %s to zip: %s", fileName, err)
		}
		rc.Close()
	}

	return nil
}

func (c ParallelCoordinator) createBundle(ctx context.Context, node node, id string, jobs chan<- job) BundleStatus {
	_, err := c.client.CreateBundle(ctx, node.baseURL, id)
	if err != nil {
		// Return done status with error. To mark node as errored so file will not be downloaded
		return BundleStatus{
			id:   id,
			node: node,
			done: true,
			err:  fmt.Errorf("could not create bundle: %s", err),
		}
	}

	// Schedule bundle status check
	jobs <- func(ctx context.Context) BundleStatus {
		return c.waitForDone(ctx, node, id, jobs)
	}

	// Return undone status with no error.
	return BundleStatus{id: id, node: node}
}

func (c ParallelCoordinator) waitForDone(ctx context.Context, node node, id string, jobs chan<- job) BundleStatus {
	select {
	case <-ctx.Done():
		return BundleStatus{
			id:   id,
			node: node,
			done: true,
			err:  errors.New(contextDoneErrMsg),
		}
	default:
	}

	statusCheck := func() {
		jobs <- func(ctx context.Context) BundleStatus {
			return c.waitForDone(ctx, node, id, jobs)
		}
	}

	logrus.WithField("IP", node.IP).Info("Checking bundle status on node.")
	// Check bundle status
	bundle, err := c.client.Status(ctx, node.baseURL, id)
	// If error
	if err != nil {
		logrus.WithField("IP", node.IP).WithError(err).Error("Error occurred checking bundle status, continuing")
		// then schedule next check in given time.
		// It will only add check to job queue so interval might increase but it's OK.
		time.AfterFunc(c.statusCheckInterval, statusCheck)
		// Return status with error. Do not mark bundle as done yet. It might change it status
		return BundleStatus{id: id, node: node, err: fmt.Errorf("could not check status: %s", err)}
	}
	// If bundle is in terminal state (its state won't change)
	if bundle.Status == Done || bundle.Status == Deleted || bundle.Status == Canceled {
		logrus.WithField("IP", node.IP).Info("Node bundle is finished.")
		// mark it as done
		return BundleStatus{id: id, node: node, done: true}
	}
	// If bundle is still in progress (InProgress, Unknown or Started)
	// then schedule next check in given time
	// It will only add check to job queue so interval might increase but it's OK.
	time.AfterFunc(c.statusCheckInterval, statusCheck)
	// Return undone status with no error. Do not mark bundle as done yet. It might change it status
	return BundleStatus{id: id, node: node}
}

func nodeBundleFilename(n node) string {
	return fmt.Sprintf("%s_%s.zip", n.IP, n.Role)
}
