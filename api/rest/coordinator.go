package rest

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

type Coordinator struct {
	client Client
}

// job is a function that will be called by worker. The output will be added to results chanel
type job func() bundleStatus

// worker is a function that will run incoming jobs from jobs channel and put jobs output to results chan
func worker(ctx context.Context, jobs <-chan job, results chan<- bundleStatus) {
	for j := range jobs {
		//TODO(janisz): Handle context
		results <- j()
	}
}

// Create starts bundle creation process. Creation process could be monitor on returned channel.
func (c Coordinator) Create(ctx context.Context, id string, nodes []node) <-chan bundleStatus {

	jobs := make(chan job)
	statuses := make(chan bundleStatus)

	for i := 0; i < 10; i++ {
		go worker(ctx, jobs, statuses)
	}

	for _, n := range nodes {
		jobs <- func() bundleStatus {
			//TODO(janisz): Handle context
			return c.createBundle(ctx, n, id, jobs)
		}
	}

	return statuses
}

func (c Coordinator) Collect(ctx context.Context, statuses <-chan bundleStatus) (bundlePath string, err error) {

	var bundles []string

	for s := range statuses {
		//TODO(janisz): Handle context

		if !s.done {
			logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Info("Got status update. Bundle not ready.")
		}
		if s.err != nil {
			logrus.WithError(s.err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Bundle errored")
		}

		bundlePath, err := c.client.GetFile(ctx, s.node, s.ID)
		if err != nil {
			logrus.WithError(err).WithField("IP", s.node.IP).WithField("ID", s.ID).Warn("Could not download file")
		}

		bundles = append(bundles, bundlePath)
	}

	//TODO(janisz): Merge all bundles into a single zip and return it
	return "", nil
}

func (c Coordinator) createBundle(ctx context.Context, node node, id string, jobs chan<- job) bundleStatus {
	_, err := c.client.Create(ctx, node, id)
	if err != nil {
		// Return done status with error. To mark node as errored so file will not be downloaded
		return bundleStatus{
			ID:   id,
			node: node,
			done: true,
			err:  fmt.Errorf("could not create bundle: %s", err),
		}
	}

	// Schedule bundle status check
	jobs <- func() bundleStatus {
		return c.waitForDone(ctx, node, id, jobs)
	}

	// Return undone status with no error.
	return bundleStatus{ID: id, node: node}
}

func (c Coordinator) waitForDone(ctx context.Context, node node, id string, jobs chan<- job) bundleStatus {
	statusCheck := func() {
		jobs <- func() bundleStatus {
			return c.waitForDone(ctx, node, id, jobs)
		}
	}

	//TODO(janisz): Handle context

	// Check bundle status
	bundle, err := c.client.Status(ctx, node, id)
	// If error
	if err != nil {
		// then schedule next check in given time.
		// It will only add check to job queue so interval might increase but it's OK.
		time.AfterFunc(time.Second, statusCheck)
		// Return status with error. Do not mark bundle as done yet. It might change it status
		return bundleStatus{ID: id, node: node, err: fmt.Errorf("could not check status: %s", err)}
	}
	// If bundle is in terminal state (its state won't change)
	if bundle.Status == Done || bundle.Status == Deleted || bundle.Status == Canceled {
		// mark it as done
		return bundleStatus{ID: id, node: node, done: true}
	}
	// If bundle is still in progress (InProgress, Unknown or Started)
	// then schedule next check in given time
	// It will only add check to job queue so interval might increase but it's OK.
	time.AfterFunc(time.Second, statusCheck)
	// Return undone status with no error. Do not mark bundle as done yet. It might change it status
	return bundleStatus{ID: id, node: node}
}
