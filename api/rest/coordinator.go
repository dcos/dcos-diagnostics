package rest

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/sirupsen/logrus"
)

const numberOfWorkers = 10

type BundleStatus struct {
	ID   string
	node node
	done bool
	err  error
}

// Coordinator is an interface to coordinate the creation of diagnostics bundles
// across a cluster of nodes
type Coordinator interface {
	Create(ctx context.Context, id string, nodes []node) <-chan BundleStatus
	Collect(ctx context.Context, bundleID string, numBundles int, statuses <-chan BundleStatus) (string, error)
}

// ParallelCoordinator implements Coordinator interface to coordinate bundle
// creation across a cluster, parallelized across multiple goroutines
type ParallelCoordinator struct {
	client Client
}

// job is a function that will be called by worker. The output will be added to results chanel
type job func() BundleStatus

// worker is a function that will run incoming jobs from jobs channel and put jobs output to results chan
func worker(ctx context.Context, jobs <-chan job, results chan<- BundleStatus) {
	for {
		select {
		case <-ctx.Done():
			break
		case j := <-jobs:
			results <- j()
		}
	}
}

// NewParallelCoordinator constructs a new parallelcoordinator.
func NewParallelCoordinator(client Client) ParallelCoordinator {
	return ParallelCoordinator{
		client: client,
	}
}

// Create starts bundle creation process. Creation process could be monitor on returned channel.
func (c ParallelCoordinator) Create(ctx context.Context, id string, nodes []node) <-chan BundleStatus {

	jobs := make(chan job)
	statuses := make(chan BundleStatus)

	for i := 0; i < numberOfWorkers; i++ {
		go worker(ctx, jobs, statuses)
	}

	for _, n := range nodes {
		logrus.WithField("IP", n.IP).Info("Sending creation request to node.")

		// necessary to prevent the closure from giving the same node to all the calls
		tmpNode := n
		jobs <- func() BundleStatus {
			//TODO(janisz): Handle context

			// because this will also make a request from the coordinating master, we
			// can't tell it to make a local bundle with the same ID this guarantees that
			// the coordinating master's local bundle has a different ID.
			// from this point, the full <ip>-<id> will be carried in the BundleStatus ID field so this does not
			// need to be recalculated in the coordinator
			fullID := fmt.Sprintf("%s-%s", tmpNode.IP, id)
			return c.createBundle(ctx, tmpNode, fullID, jobs)
		}
	}

	return statuses
}

func (c ParallelCoordinator) Collect(ctx context.Context, bundleID string, numBundles int, statuses <-chan BundleStatus) (string, error) {

	var bundles []string

	for len(bundles) < numBundles {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context canceled before all node bundles finished")
		case s := <-statuses:
			if !s.done {
				logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Info("Got status update. Bundle not ready.")
				continue
			}
			if s.err != nil {
				logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Bundle errored")
				// TODO: the bundle will never finish, need to stop this from waiting forever
				continue
			}

			//TODO(janisz): Handle file creation in workdir
			// the '*' will be swapped out with a random string by ioutil.TempFile
			// This will use the default temp directory from os.TempDir
			destinationFile, err := ioutil.TempFile("", fmt.Sprintf("bundle-%s-*.zip", bundleID))
			if err != nil {
				return "", fmt.Errorf("could not create temporary result file")
			}
			_ = destinationFile.Close()

			err = c.client.GetFile(ctx, s.node.baseURL, s.ID, destinationFile.Name())
			if err != nil {
				logrus.WithError(err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Could not download file")
			}

			bundles = append(bundles, destinationFile.Name())
		}
	}

	return mergeZips(bundleID, bundles)
}

func mergeZips(bundleID string, bundlePaths []string) (string, error) {

	bundleName := fmt.Sprintf("bundle-%s-*.zip", bundleID)
	mergedZip, err := ioutil.TempFile("", bundleName)
	if err != nil {
		return "", err
	}
	defer mergedZip.Close()

	zipWriter := zip.NewWriter(mergedZip)
	defer zipWriter.Close()

	for _, p := range bundlePaths {
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

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("could not open %s from zip: %s", f.Name, err)
		}

		file, err := writer.Create(f.Name)
		if err != nil {
			return fmt.Errorf("could not create file %s: %s", f.Name, err)
		}
		_, err = io.Copy(file, rc)
		if err != nil {
			return fmt.Errorf("could not copy file %s to zip: %s", f.Name, err)
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
			ID:   id,
			node: node,
			done: true,
			err:  fmt.Errorf("could not create bundle: %s", err),
		}
	}

	// Schedule bundle status check
	jobs <- func() BundleStatus {
		return c.waitForDone(ctx, node, id, jobs)
	}

	// Return undone status with no error.
	return BundleStatus{ID: id, node: node}
}

func (c ParallelCoordinator) waitForDone(ctx context.Context, node node, id string, jobs chan<- job) BundleStatus {
	statusCheck := func() {
		jobs <- func() BundleStatus {
			return c.waitForDone(ctx, node, id, jobs)
		}
	}

	//TODO(janisz): Handle context

	logrus.WithField("IP", node.IP).Info("Checking bundle status on node.")
	// Check bundle status
	bundle, err := c.client.Status(ctx, node.baseURL, id)
	// If error
	if err != nil {
		// then schedule next check in given time.
		// It will only add check to job queue so interval might increase but it's OK.
		time.AfterFunc(time.Second, statusCheck)
		// Return status with error. Do not mark bundle as done yet. It might change it status
		return BundleStatus{ID: id, node: node, err: fmt.Errorf("could not check status: %s", err)}
	}
	// If bundle is in terminal state (its state won't change)
	if bundle.Status == Done || bundle.Status == Deleted || bundle.Status == Canceled {
		logrus.WithField("IP", node.IP).Info("Node bundle is finished.")
		// mark it as done
		return BundleStatus{ID: id, node: node, done: true}
	}
	// If bundle is still in progress (InProgress, Unknown or Started)
	// then schedule next check in given time
	// It will only add check to job queue so interval might increase but it's OK.
	time.AfterFunc(time.Second, statusCheck)
	// Return undone status with no error. Do not mark bundle as done yet. It might change it status
	return BundleStatus{ID: id, node: node}
}
