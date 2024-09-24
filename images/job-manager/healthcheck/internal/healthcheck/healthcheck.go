// Copyright 2023-2024 The MathWorks, Inc.
package healthcheck

import (
	"errors"
	"fmt"
	"mjshealthcheck/internal/status"
)

type HealthChecker struct {
	statusGetter status.StatusGetter
}

func NewHealthChecker(statusGetter status.StatusGetter) *HealthChecker {
	return &HealthChecker{
		statusGetter: statusGetter,
	}
}

// Return true if a job manager is found and healthy. If the healthcheck fails, return a diagnostic message.
func (h *HealthChecker) DoJobManagerHealthcheck(jobManagerName string) (bool, string, error) {
	jobManagers, err := h.statusGetter.GetJobManagers()
	if err != nil {
		return false, "", err
	}
	if len(jobManagers) == 0 {
		return false, "No job managers found", nil
	}
	status := ""
	if jobManagerName == "" {
		if len(jobManagers) > 1 {
			return false, "", errors.New("error: multiple job managers were found; a job manager name must be specified in order to perform a healthcheck")
		}
		// If a job manager name was not specified, use the only job manager
		status = jobManagers[0].Status
	} else {
		// If a specific job manager name was specified, check that specific job manager
		found := false
		status, found = findJobManagerStatus(jobManagerName, jobManagers)
		if !found {
			return false, fmt.Sprintf("Job manager \"%s\" not found", jobManagerName), nil
		}
	}
	isHealthy, msg := isHealthyStatus(status)
	return isHealthy, msg, nil
}

// Return true if a worker group is running. If healthcheck fails, return a diagnostic message.
func (h *HealthChecker) DoWorkerGroupHealthcheck() (bool, string, error) {
	status, err := h.statusGetter.GetWorkerGroupStatus()
	if err != nil {
		return false, "", err
	}
	isHealthy := status == "Running"
	msg := ""
	if !isHealthy {
		msg = fmt.Sprintf("Worker group status: %s", status)
	}
	return isHealthy, msg, nil
}

// Find the status of a job manager with a given name
func findJobManagerStatus(name string, jobManagers []status.JobManagerStatus) (string, bool) {
	for _, jm := range jobManagers {
		if jm.Name == name {
			return jm.Status, true
		}
	}
	return "", false
}

const statusRunning = "running"
const statusPaused = "paused"

func isHealthyStatus(status string) (bool, string) {
	isHealthy := status == statusRunning || status == statusPaused
	msg := ""
	if !isHealthy {
		msg = fmt.Sprintf("Job manager status: %s", status)
	}
	return isHealthy, msg
}
