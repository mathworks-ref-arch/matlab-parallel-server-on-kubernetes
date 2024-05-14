// Package request contains code for getting a cluster's resize request
// Copyright 2024 The MathWorks, Inc.
package request

import (
	"bytes"
	"controller/internal/config"
	"controller/internal/k8s"
	"controller/internal/logging"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Getter is an interface for getting a cluster's resize request
type Getter interface {
	GetRequest() (*ResizeRequest, error)
}

// MJSRequestGetter implements the Getter interface for an MJS cluster in Kubernetes
type MJSRequestGetter struct {
	config   *config.Config
	logger   *logging.Logger
	client   k8s.Client
	exitFunc func()
}

// NewMJSRequestGetter constructs an MJSRequestGetter
func NewMJSRequestGetter(conf *config.Config, logger *logging.Logger) (*MJSRequestGetter, error) {
	client, err := k8s.NewClient(conf, logger)
	if err != nil {
		return nil, err
	}

	m := &MJSRequestGetter{
		config:   conf,
		logger:   logger,
		client:   client,
		exitFunc: func() { os.Exit(1) },
	}
	return m, nil
}

// GetRequest excutes the resize script on the job manager to obtain the cluster's resize request
func (m *MJSRequestGetter) GetRequest() (*ResizeRequest, error) {
	rawStatus, err := m.execResizeOnJobManager()
	if err != nil {
		return nil, fmt.Errorf("error getting resize status via Kubernetes exec: %v", err)
	}
	return m.processRequest(rawStatus.Bytes())
}

// execResizeOnJobManager uses the Kubernetes RESTful interface to execute the resize script on the job manager
func (m *MJSRequestGetter) execResizeOnJobManager() (*bytes.Buffer, error) {
	// Create command to run
	cmd := getResizeStatusCommand(m.config)

	// Find the job manager pod so we can extract its name; note that the name may change over time if the pod is restarted
	pod, err := m.client.GetJobManagerPod()
	if err != nil {
		return nil, err
	}

	// Execute the command
	return m.client.ExecOnPod(pod.Name, cmd)
}

// processRequest converts the raw output of "./resize status" into a ResizeRequest struct
func (m *MJSRequestGetter) processRequest(input []byte) (*ResizeRequest, error) {
	// Convert the status bytes to a struct
	rawStatus := resizeStatus{}
	err := json.Unmarshal(input, &rawStatus)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from resize status output. Error: %v. Raw output: \"%s\"", err, string(input))
	}

	// Extract the resize request from the status struct
	numJobManagers := len(rawStatus.JobManagers)
	if numJobManagers == 0 {
		return nil, errors.New("no job managers found running in the job manager pod")
	}
	if numJobManagers > 1 {
		// A previous job manager was drooled in the checkpoint base, so we should error
		jobManagerNames := []string{}
		for _, jm := range rawStatus.JobManagers {
			jobManagerNames = append(jobManagerNames, jm.Name)
		}
		msg := fmt.Sprintf("Multiple job managers were found running in the job manager pod: %v. This happens when a process from a previous job manager remains in the checkpoint base folder. Uninstall MATLAB Job Scheduler from the Kubernetes cluster, delete the contents of the checkpoint base folder, then try again.", jobManagerNames)
		m.logger.Error(msg)
		fmt.Println(msg)
		m.exitFunc()
	}
	jobManagerStatus := rawStatus.JobManagers[0]
	req := ResizeRequest{}
	req.DesiredWorkers = jobManagerStatus.DesiredWorkers.Linux
	req.MaxWorkers = jobManagerStatus.MaxWorkers.Linux
	req.Workers = jobManagerStatus.Workers
	return &req, nil
}

// ResizeRequest represents a cluster's current and requested number of workers
type ResizeRequest struct {
	DesiredWorkers int
	MaxWorkers     int
	Workers        []WorkerStatus
}

// resizeStatus is a struct matching the format of the output of "./resize status"
type resizeStatus struct {
	JobManagers []jobManagerStatus
}

// jobManagerStatus is a struct matching the "JobManagers" field in the output of "./resize status"
type jobManagerStatus struct {
	Name           string
	DesiredWorkers workersPerOS
	MaxWorkers     workersPerOS
	Workers        []WorkerStatus
}

// WorkerStatus is a struct matching the "Workers" field in the job manager output of "./resize status"
type WorkerStatus struct {
	Name        string
	SecondsIdle int
	State       string
}

// workersPerOS is a struct matching the output of the DesiredWorkers and MaxWorkers fields in the job manager output of "./resize status"
type workersPerOS struct {
	Linux   int
	Windows int
}

func getResizeStatusCommand(conf *config.Config) []string {
	timeout := k8s.Timeout - 5 // Use a timeout shorter than the Kubernetes client timeout so we don't leave an orphaned process on the pod
	cmd := []string{"timeout", fmt.Sprintf("%d", timeout), conf.ResizePath, "status", "-baseport", fmt.Sprintf("%d", conf.BasePort)}
	if conf.RequireScriptVerification {
		cmd = append(cmd, "-secretfile", filepath.Join(conf.SecretDir, conf.SecretFileName))
	}
	return cmd
}
