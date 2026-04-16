// Copyright 2024-2025 The MathWorks, Inc.
package controller_test

import (
	"controller/internal/controller"
	"controller/internal/logging"
	mocks "controller/mocks/controller"
	"errors"
	"slices"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestIncreaseWorkers(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add an existing worker
	existingWorkerName := "worker1"
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return([]controller.DeployedWorker{
		{
			ID:   1,
			Name: existingWorkerName,
		},
	}, nil).Once()

	// Job manager wants more workers
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 2,
		Workers: []controller.ConnectedWorker{
			{
				Name: existingWorkerName,
			},
		},
	}, nil).Once()

	// Expect a worker to be added
	mocks.resizer.EXPECT().AddWorkers([]int{2}).Return(nil).Once()
	c.Run()
}

func TestMinWorkers(t *testing.T) {
	minWorkers := 5
	c, mocks := newWithMocks(t, minWorkers)

	// Start with no workers
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return([]controller.DeployedWorker{}, nil).Once()

	// Job manager doesn't want any workers
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        []controller.ConnectedWorker{},
	}, nil).Once()

	// Still expect workers to be added to make minWorkers
	toAdd := []int{}
	for i := 1; i <= minWorkers; i++ {
		toAdd = append(toAdd, i)
	}
	mocks.resizer.EXPECT().AddWorkers(toAdd).Return(nil).Once()
	c.Run()
}

func TestDecreaseWorkers(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add some existing workers
	existingWorkers := []controller.DeployedWorker{
		{
			Name: "worker1",
			ID:   1,
		},
		{
			Name: "worker2",
			ID:   2,
		},
	}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(existingWorkers, nil).Once()

	// Job manager wants fewer workers
	connectedWorkers := []controller.ConnectedWorker{}
	for _, worker := range existingWorkers[:2] {
		connectedWorkers = append(connectedWorkers, controller.ConnectedWorker{
			Name:        worker.Name,
			State:       "idle",
			SecondsIdle: idleStop + 1, // Workers can be stopped
		})
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        connectedWorkers,
	}, nil).Once()

	// Expect workers to be deleted
	// Note we check workers for deletion in reverse order of ID
	slices.Reverse(existingWorkers)
	mocks.resizer.EXPECT().DeleteWorkers(existingWorkers).Return(nil).Once()
	c.Run()
}

func TestWorkersAlreadyMax(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add some existing workers
	existingWorkers := []controller.DeployedWorker{
		{
			Name: "worker1",
			ID:   1,
		},
		{
			Name: "worker2",
			ID:   2,
		},
	}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(existingWorkers, nil).Once()

	// Job manager already has its max workers
	connectedWorkers := []controller.ConnectedWorker{}
	for _, worker := range existingWorkers[:2] {
		connectedWorkers = append(connectedWorkers, controller.ConnectedWorker{
			Name:  worker.Name,
			State: "running",
		})
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     2,
		DesiredWorkers: 2,
		Workers:        connectedWorkers,
	}, nil).Once()

	// No changes expected
	c.Run()
}

func TestWorkersNotIdle(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add some existing workers
	existingWorkers := []controller.DeployedWorker{
		{
			Name: "worker1",
			ID:   1,
		},
		{
			Name: "worker2",
			ID:   2,
		},
	}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(existingWorkers, nil).Once()

	// Job manager wants fewer workers, but all workers are still busy
	connectedWorkers := []controller.ConnectedWorker{}
	for _, worker := range existingWorkers[:2] {
		connectedWorkers = append(connectedWorkers, controller.ConnectedWorker{
			Name:  worker.Name,
			State: "busy",
		})
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        connectedWorkers,
	}, nil).Once()

	// Expect no changes
	c.Run()
}

func TestWorkersNotIdleForLongEnough(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add some existing workers
	existingWorkers := []controller.DeployedWorker{
		{
			Name: "worker1",
			ID:   1,
		},
		{
			Name: "worker2",
			ID:   2,
		},
	}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(existingWorkers, nil).Once()

	// Job manager wants fewer workers, but none of the workers have been idle for long enough
	connectedWorkers := []controller.ConnectedWorker{}
	for _, worker := range existingWorkers[:2] {
		connectedWorkers = append(connectedWorkers, controller.ConnectedWorker{
			Name:        worker.Name,
			State:       "idle",
			SecondsIdle: idleStop - 1, // Workers cannot be stopped
		})
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        connectedWorkers,
	}, nil).Once()

	// Expect no changes
	c.Run()
}

// Check the expected worker gets deleted when there are multiple workers
func TestWorkersOneDeletionCandidate(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Add some existing workers
	runningWorker := controller.DeployedWorker{
		Name: "worker1",
		ID:   1,
	}
	shortIdleWorker := controller.DeployedWorker{
		Name: "worker2",
		ID:   2,
	}
	stoppableWorker := controller.DeployedWorker{
		Name: "worker3",
		ID:   3,
	}
	existingWorkers := []controller.DeployedWorker{runningWorker, shortIdleWorker, stoppableWorker}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(existingWorkers, nil).Once()

	// Job manager wants fewer workers, but only one can be deleted
	connectedWorkers := []controller.ConnectedWorker{
		{
			Name:  runningWorker.Name,
			State: "running", // Worker can't be stopped
		},
		{
			Name:        shortIdleWorker.Name,
			State:       "idle",
			SecondsIdle: idleStop - 5, // Worker can't be stopped
		},
		{
			Name:        stoppableWorker.Name,
			State:       "idle",
			SecondsIdle: idleStop + 1, // Worker can be stopped
		},
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 1,
		Workers:        connectedWorkers,
	}, nil).Once()

	// Expect only one worker to be removed
	mocks.resizer.EXPECT().DeleteWorkers([]controller.DeployedWorker{stoppableWorker}).Return(nil).Once()
	c.Run()
}

// We should not delete workers that haven't connected to the job manager yet (g3278906)
func TestWorkersNotConnectedYet(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Create a worker that exists in K8s but is not known to the job manager
	worker := controller.DeployedWorker{
		Name: "myworker",
		ID:   1,
	}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return([]controller.DeployedWorker{worker}, nil).Once()

	// Job manager does not need any workers, and also doesn't know about any workers
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        []controller.ConnectedWorker{},
	}, nil).Once()

	// Do not expect this worker to be deleted
	c.Run()
}

// If a worker is connected to the job manager but not found in K8s, we should assume it
// has already been deleted and not try to delete it again
func TestWorkerConnectedNotDeployed(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// Expect to see only one deployed worker
	deployedWorker := controller.DeployedWorker{
		Name: "worker-in-k8s",
		ID:   1,
	}
	deployedWorkers := []controller.DeployedWorker{deployedWorker}
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(deployedWorkers, nil).Once()

	// Job manager knows about this worker plus another
	connectedWorkers := []controller.ConnectedWorker{
		{
			Name:        deployedWorker.Name,
			State:       "idle",
			SecondsIdle: idleStop + 1,
		},
		{
			Name:        "other-worker",
			State:       "idle",
			SecondsIdle: idleStop + 10,
		},
	}
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers:        connectedWorkers,
	}, nil).Once()

	// Expect only the deployed worker to be deleted
	mocks.resizer.EXPECT().DeleteWorkers(deployedWorkers).Return(nil).Once()
	c.Run()
}

// If we see workers registered with the job manager that do not exist in K8s, we should not try to delete them, as they may have already been deleted
func TestWorkersConnectedButNotDeployed(t *testing.T) {
	c, mocks := newWithMocks(t, 0)

	// K8s doesn't know about any workers
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return([]controller.DeployedWorker{}, nil).Once()

	// Job manager has a worker registered that K8s doesn't know about
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(&controller.ResizeRequest{
		MaxWorkers:     10,
		DesiredWorkers: 0,
		Workers: []controller.ConnectedWorker{
			{
				Name:        "mystery-worker",
				State:       "idle",
				SecondsIdle: idleStop + 1,
			},
		},
	}, nil).Once()

	// Do not expect this worker to be deleted
	c.Run()
}

func TestResizeRequestError(t *testing.T) {
	c, mocks := newWithMocks(t, 0)
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return([]controller.DeployedWorker{}, nil).Once()
	mocks.resizeRequestGetter.EXPECT().GetRequest().Return(nil, errors.New("failed")).Once()

	// Controller loop should return with no further calls
	c.Run()
}

func TestGetAndUpdateWorkersError(t *testing.T) {
	c, mocks := newWithMocks(t, 0)
	mocks.resizer.EXPECT().GetAndUpdateWorkers().Return(nil, errors.New("failed to get workers")).Once()

	// Controller loop should return with no further calls
	c.Run()
}

type controllerMocks struct {
	resizer             *mocks.MockResizer
	resizeRequestGetter *mocks.MockResizeRequestGetter
}

const idleStop = 30

func newWithMocks(t *testing.T, minWorkers int) (*controller.Controller, *controllerMocks) {
	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))
	mockResizer := mocks.NewMockResizer(t)
	mockRequestGetter := mocks.NewMockResizeRequestGetter(t)
	return controller.New(idleStop, minWorkers, mockRequestGetter, mockResizer, logger), &controllerMocks{
		resizer:             mockResizer,
		resizeRequestGetter: mockRequestGetter,
	}
}
