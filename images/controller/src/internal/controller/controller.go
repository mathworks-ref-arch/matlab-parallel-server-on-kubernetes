// Define the logic of the controller loop.
// This reconciles the desired MJS workers with the current workers in Kubernetes.
// Copyright 2024-2025 The MathWorks, Inc.
package controller

import (
	"controller/internal/logging"

	"go.uber.org/zap"
)

// Controller runs the controller loop logic
type Controller struct {
	logger              *logging.Logger
	idleStopThreshold   int                 // Time after which idle workers can be stopped
	minWorkers          int                 // Minimum number of workers in the cluster at any time
	resizeRequestGetter ResizeRequestGetter // Interface to get the cluster's resize request
	resizer             Resizer             // Interface to perform cluster resizing
}

// Resize is an interface for resizing a cluster
type Resizer interface {
	GetAndUpdateWorkers() ([]DeployedWorker, error)
	AddWorkers([]int) error
	DeleteWorkers([]DeployedWorker) error
}

// DeployedWorker represents a worker currently deployed in the Kubernetes cluster
type DeployedWorker struct {
	PodName   string // Name of the pod running this worker. Worker names can be repeated as the cluster scales up and down, but pod names are unique
	ID        int
	IsRunning bool   // Whether a worker pod is running
	Name      string // The name this worker uses when it connects to the job manager
}

// ResizeRequestGetter is an interface for getting a cluster's resize request
type ResizeRequestGetter interface {
	GetRequest() (*ResizeRequest, error)
}

// ResizeRequest represents a cluster's current and requested number of workers
type ResizeRequest struct {
	DesiredWorkers int
	MaxWorkers     int
	Workers        []ConnectedWorker
}

// ConnectedWorker represents a worker that is connected to the job manager
type ConnectedWorker struct {
	Name        string
	SecondsIdle int
	State       string
}

func New(idleStop, minWorkers int, resizeReqGetter ResizeRequestGetter, resizer Resizer, logger *logging.Logger) *Controller {
	return &Controller{
		idleStopThreshold:   idleStop,
		logger:              logger,
		minWorkers:          minWorkers,
		resizeRequestGetter: resizeReqGetter,
		resizer:             resizer,
	}
}

// Run a single iteration of the controller loop
func (c *Controller) Run() {
	// Get a list of the workers currently deployed into K8s, and clean up any workers that are out of date with respect to
	// the current Helm chart.
	// This gives us the "true" worker count - these workers have either already connected to MJS, or will eventually connect.
	// Any MJS worker not in this list must have already had its deployment deleted, so will soon leave the MJS cluster.
	workersInK8s, err := c.resizer.GetAndUpdateWorkers()
	if err != nil {
		c.logger.Error("Error getting existing workers from Kubernetes cluster", zap.Error(err))
		return
	}
	numWorkers := len(workersInK8s)

	// Get the cluster's resize request
	status, err := c.resizeRequestGetter.GetRequest()
	if err != nil {
		c.logger.Error("Error getting resize status", zap.Error(err))
		return
	}

	// Calculate the desired number of workers
	desiredWorkers := status.DesiredWorkers
	if desiredWorkers > status.MaxWorkers {
		// Note that this scenario should never happen with MJS, so log an error
		c.logger.Error("Desired workers should not be larger than max workers", zap.Int("maxWorkers", status.MaxWorkers), zap.Int("desiredWorkers", desiredWorkers))
		desiredWorkers = status.MaxWorkers
	} else if desiredWorkers < c.minWorkers {
		desiredWorkers = c.minWorkers
	}
	if numWorkers == desiredWorkers {
		return
	}

	if desiredWorkers < numWorkers {
		c.logger.Debug("Reducing number of workers", zap.Int("currentWorkers", numWorkers), zap.Int("desiredWorkers", desiredWorkers))
		toDelete := getWorkersToDelete(numWorkers-desiredWorkers, status.Workers, workersInK8s, c.idleStopThreshold)
		if len(toDelete) == 0 {
			c.logger.Debug("Did not find any workers available to delete")
			return
		}
		err = c.resizer.DeleteWorkers(toDelete)
		if err != nil {
			c.logger.Error("Error scaling down cluster", zap.Error(err))
		}
	} else {
		c.logger.Debug("Increasing number of workers", zap.Int("currentWorkers", numWorkers), zap.Int("desiredWorkers", desiredWorkers))
		toAdd := getWorkersToAdd(desiredWorkers, workersInK8s)
		err = c.resizer.AddWorkers(toAdd)
		if err != nil {
			c.logger.Error("Error scaling up cluster", zap.Error(err))
		}
	}
}

// getWorkersToDelete returns a list of workers that should be deleted from Kubernetes
func getWorkersToDelete(numToDelete int, connectedWorkers []ConnectedWorker, deployedWorkers []DeployedWorker, idleStopThreshold int) []DeployedWorker {
	toDelete := []DeployedWorker{}
	connectedAndDeployed := getConnectedAndDeployed(connectedWorkers, deployedWorkers)

	// Look for connected workers that have been idle for longer than the idleStopThreshold
	for i := len(connectedWorkers) - 1; i >= 0; i-- {
		w := connectedWorkers[i]

		// If a worker is no longer present in K8s, it must already be in the process of terminating, so don't try to delete it again
		deployedWorker, isDeployed := connectedAndDeployed[w.Name]
		if !isDeployed {
			continue
		}

		shouldDelete := w.State == "idle" && w.SecondsIdle >= idleStopThreshold
		if shouldDelete {
			toDelete = append(toDelete, deployedWorker)
			if len(toDelete) == numToDelete {
				return toDelete
			}
		}
	}
	return toDelete
}

// Return a map containing deployed workers that are also connected to the job manager
func getConnectedAndDeployed(connectedWorkers []ConnectedWorker, deployedWorkers []DeployedWorker) map[string]DeployedWorker {
	connectedAndDeployed := map[string]DeployedWorker{}

	// Create has map of connected worker names for efficient lookup
	connectedNames := map[string]bool{}
	for _, w := range connectedWorkers {
		connectedNames[w.Name] = true
	}

	// Loop through deployed workers and store in the map if they're also connected to the job manager
	for _, w := range deployedWorkers {
		isConnected := connectedNames[w.Name]
		if isConnected {
			connectedAndDeployed[w.Name] = w
		}
	}

	return connectedAndDeployed
}

// getWorkersToAdd computes which worker IDs should be added, starting from the lowest available worker ID, such that we have the desired number of workers
func getWorkersToAdd(desiredWorkers int, existingWorkers []DeployedWorker) []int {
	newWorkers := []int{}
	id := 1
	numToAdd := desiredWorkers - len(existingWorkers)

	// Create hash map of existing worker IDs
	existingIDs := map[int]bool{}
	for _, w := range existingWorkers {
		existingIDs[w.ID] = true
	}

	for len(newWorkers) < numToAdd {
		if !existingIDs[id] {
			newWorkers = append(newWorkers, id)
		}
		id++
	}
	return newWorkers
}
