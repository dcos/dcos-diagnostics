package rest

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

const numberOfWorkers = 10

type Coordinator struct {
	client Client
}

// job is a function that will be called by worker. The output will be added to results chanel
type job func() BundleStatus

// worker is a function that will run incoming jobs from jobs channel and put jobs output to results chan
func worker(_ context.Context, jobs <-chan job, results chan<- BundleStatus) {
	for j := range jobs {
		//TODO(janisz): Handle context
		results <- j()
	}
}

// Create starts bundle creation process. Creation process could be monitor on returned channel.
func (c Coordinator) Create(ctx context.Context, id string, nodes []node) <-chan BundleStatus {

	jobs := make(chan job)
	statuses := make(chan BundleStatus)

	for i := 0; i < numberOfWorkers; i++ {
		go worker(ctx, jobs, statuses)
	}

	for _, n := range nodes {
		logrus.WithField("IP", n.IP).Info("Sending creation request to node.")
		jobs <- func() BundleStatus {
			//TODO(janisz): Handle context
			return c.createBundle(ctx, n, id, jobs)
		}
	}

	return statuses
}

func (c Coordinator) Collect(ctx context.Context, statuses <-chan BundleStatus) (bundlePath string, err error) {

	var bundles []string

	for s := range statuses {
		//TODO(janisz): Handle context

		if !s.done {
			logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Info("Got status update. Bundle not ready.")
		}
		if s.err != nil {
			logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Bundle errored")
		}

		bundlePath, err := c.client.GetFile(ctx, s.node.baseURL, s.ID)
		if err != nil {
			logrus.WithError(err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Could not download file")
		}

		bundles = append(bundles, bundlePath)
	}

	//TODO(janisz): Merge all bundles into a single zip and return it
	return "", nil
}

func (c Coordinator) createBundle(ctx context.Context, node node, id string, jobs chan<- job) BundleStatus {
	_, err := c.client.Create(ctx, node.baseURL, id)
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

func (c Coordinator) waitForDone(ctx context.Context, node node, id string, jobs chan<- job) BundleStatus {
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
