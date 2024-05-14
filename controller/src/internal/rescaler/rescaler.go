// Package rescaler contains logic for rescaling an MJS cluster in Kubernetes base on its resize status.
// Copyright 2024 The MathWorks, Inc.
package rescaler

import (
	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/request"
	"controller/internal/resize"
	"controller/internal/specs"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
)

// Rescaler is an interface for performing cluster rescaling
type Rescaler interface {
	Rescale()
}

// MJSRescaler implements the Rescaler interface to rescale an MJS cluster in Kubernetes
type MJSRescaler struct {
	logger            *logging.Logger
	idleStopThreshold int            // Time after which idle workers can be stopped
	minWorkers        int            // Minimum number of workers in the cluster at any time
	requestGetter     request.Getter // Interface to get the cluster's resize request
	resizer           resize.Resizer // Interface to perform cluster resizing
}

// NewMJSRescaler constructs an MJSRescaler from a given config struct
func NewMJSRescaler(config *config.Config, ownerUID types.UID, logger *logging.Logger) (*MJSRescaler, error) {
	requestGetter, err := request.NewMJSRequestGetter(config, logger)
	if err != nil {
		return nil, fmt.Errorf("error creating K8s resize status getter: %v", err)
	}

	resizer, err := resize.NewMJSResizer(config, ownerUID, logger)
	if err != nil {
		return nil, fmt.Errorf("error creating K8s cluster resizer: %v", err)
	}

	scaler := &MJSRescaler{
		idleStopThreshold: config.IdleStop,
		logger:            logger,
		minWorkers:        config.MinWorkers,
		requestGetter:     requestGetter,
		resizer:           resizer,
	}
	return scaler, nil
}

// Rescale gets the MJS cluster's resize status and resizes the cluster if needed
func (m *MJSRescaler) Rescale() {
	// Get the cluster's resize request
	status, err := m.requestGetter.GetRequest()
	if err != nil {
		m.logger.Error("Error getting resize status", zap.Error(err))
		return
	}

	// Calculate the desired number of workers
	desiredWorkers := status.DesiredWorkers
	if desiredWorkers > status.MaxWorkers {
		// Note that this scenario should never happen with MJS, so log an error
		m.logger.Error("Desired workers should not be larger than max workers", zap.Int("maxWorkers", status.MaxWorkers), zap.Int("desiredWorkers", desiredWorkers))
		desiredWorkers = status.MaxWorkers
	} else if desiredWorkers < m.minWorkers {
		desiredWorkers = m.minWorkers
	}

	// Get a list of the workers currently deployed into K8s.
	// This gives us the "true" worker count - these workers have either already connected to MJS, or will eventually connect.
	// Any MJS worker not in this list must have already had its deployment deleted, so will soon leave the MJS cluster.
	workersInK8s, err := m.resizer.GetWorkers()
	numWorkers := len(workersInK8s)
	if err != nil {
		m.logger.Error("Error getting existing workers from Kubernetes cluster", zap.Error(err))
		return
	}
	if numWorkers == desiredWorkers {
		return
	}

	if desiredWorkers < numWorkers {
		m.logger.Debug("Reducing number of workers", zap.Int("currentWorkers", numWorkers), zap.Int("desiredWorkers", desiredWorkers))
		toDelete := getWorkersToDelete(numWorkers-desiredWorkers, status.Workers, workersInK8s, m.idleStopThreshold)
		if len(toDelete) == 0 {
			m.logger.Debug("Did not find any workers available to delete")
			return
		}
		err = m.resizer.DeleteWorkers(toDelete)
		if err != nil {
			m.logger.Error("Error scaling down cluster", zap.Error(err))
		}
	} else {
		m.logger.Debug("Increasing number of workers", zap.Int("currentWorkers", numWorkers), zap.Int("desiredWorkers", desiredWorkers))
		toAdd := getWorkersToAdd(desiredWorkers, workersInK8s)
		err = m.resizer.AddWorkers(toAdd)
		if err != nil {
			m.logger.Error("Error scaling up cluster", zap.Error(err))
		}
	}
}

// getWorkersToDelete returns a list of workers that should be deleted
func getWorkersToDelete(numToDelete int, connectedWorkers []request.WorkerStatus, workersInK8s []resize.Worker, idleStopThreshold int) []string {
	toDelete := []string{}
	inBothLists := getWorkerOverlap(connectedWorkers, workersInK8s)

	// Look for connected workers that have been idle for longer than the idleStopThreshold
	for i := len(connectedWorkers) - 1; i >= 0; i-- {
		w := connectedWorkers[i]

		// If a worker is no longer present in K8s, it must already be in the process of terminating, so don't try to delete it again
		if !inBothLists[w.Name] {
			continue
		}

		shouldDelete := w.State == "idle" && w.SecondsIdle >= idleStopThreshold
		if shouldDelete {
			toDelete = append(toDelete, w.Name)
			if len(toDelete) == numToDelete {
				return toDelete
			}
		}
	}
	return toDelete
}

// getWorkerOverlap returns a map of booleans indicating whether a worker appears in both the list of workers connected to MJS and the list of workers connected to the cluster
func getWorkerOverlap(connectedWorkers []request.WorkerStatus, workersInK8s []resize.Worker) map[string]bool {
	isInBoth := map[string]bool{}

	// Insert all of the connected worker names into the map
	for _, w := range connectedWorkers {
		isInBoth[w.Name] = false
	}

	// Insert all of the K8s workers, flipping the value to "true" if the worker is already in the map
	for _, w := range workersInK8s {
		_, inFirstList := isInBoth[w.Info.Name]
		isInBoth[w.Info.Name] = inFirstList
	}

	return isInBoth
}

// getWorkersToAdd computes which workers should be added, starting from the lowest available worker ID, such that we have the desired number of workers
func getWorkersToAdd(desiredWorkers int, workers []resize.Worker) []specs.WorkerInfo {
	newWorkers := []specs.WorkerInfo{}
	id := 1
	numToAdd := desiredWorkers - len(workers)

	// Create hash map of existing worker names
	existingNames := map[string]bool{}
	for _, w := range workers {
		existingNames[w.Info.Name] = true
	}

	for len(newWorkers) < numToAdd {
		name := workerIDToName(id)
		if !existingNames[name] {
			newWorkers = append(newWorkers, specs.WorkerInfo{
				Name: name,
				ID:   id,
			})
		}
		id++
	}
	return newWorkers
}

// workerIDToName converts a worker ID to a worker name
func workerIDToName(id int) string {
	return fmt.Sprintf("mjs-worker-%d", id)
}
