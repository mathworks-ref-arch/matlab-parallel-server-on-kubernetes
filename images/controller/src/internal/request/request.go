// Implement the ability to get the current and desired worker count from the job manager
// by executing a command on the pod.
// Copyright 2024-2025 The MathWorks, Inc.
package request

import (
	"bytes"
	"controller/internal/controller"
	"controller/internal/k8s"
	"controller/internal/logging"
	"encoding/json"
	"errors"
	"fmt"
)

// PodResizeRequestGetter implements ResizeRequestGetter by executing the "resize status"
// command on the job manager pod to get the job manager's desired workers.
type PodResizeRequestGetter struct {
	resizeStatusCommand []string
	jobManagerContainer string
	jobManagerLabel     string
	logger              *logging.Logger
	client              k8s.Client
	exitFunc            func(error)
}

type ExecConfig struct {
	BasePort                  int
	JobManagerContainer       string
	JobManagerLabel           string
	JobManagerName            string
	RequireScriptVerification bool
	ResizePath                string
	SecretPath                string
	TimeoutSecs               int
}

func NewPodResizeRequestGetter(conf ExecConfig, client k8s.Client, exitFunc func(error), logger *logging.Logger) controller.ResizeRequestGetter {
	return &PodResizeRequestGetter{
		resizeStatusCommand: GetResizeStatusCommand(conf),
		jobManagerContainer: conf.JobManagerContainer,
		jobManagerLabel:     conf.JobManagerLabel,
		logger:              logger,
		client:              client,
		exitFunc:            exitFunc,
	}
}

// GetRequest excutes the resize script on the job manager to obtain the cluster's resize request
func (r *PodResizeRequestGetter) GetRequest() (*controller.ResizeRequest, error) {
	rawStatus, err := r.execResizeOnJobManager()
	if err != nil {
		return nil, fmt.Errorf("error getting resize status via Kubernetes exec: %v", err)
	}
	return r.processRequest(rawStatus.Bytes())
}

// execResizeOnJobManager uses the Kubernetes RESTful interface to execute the resize script on the job manager
func (r *PodResizeRequestGetter) execResizeOnJobManager() (*bytes.Buffer, error) {
	// Find the job manager pod so we can extract its name; note that the name may change over time if the pod is restarted
	pod, err := r.client.GetReadyPod(r.jobManagerLabel, r.jobManagerContainer)
	if err != nil {
		return nil, err
	}

	// Execute the command
	return r.client.ExecOnPod(pod.Name, r.jobManagerContainer, r.resizeStatusCommand)
}

// processRequest converts the raw output of "./resize status" into a ResizeRequest struct
func (r *PodResizeRequestGetter) processRequest(input []byte) (*controller.ResizeRequest, error) {
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
		r.exitFunc(errors.New(msg))
	}
	jobManagerStatus := rawStatus.JobManagers[0]
	req := controller.ResizeRequest{}
	req.DesiredWorkers = jobManagerStatus.DesiredWorkers.Linux
	req.MaxWorkers = jobManagerStatus.MaxWorkers.Linux
	req.Workers = jobManagerStatus.Workers
	return &req, nil
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
	Workers        []controller.ConnectedWorker
}

// workersPerOS is a struct matching the output of the DesiredWorkers and MaxWorkers fields in the job manager output of "./resize status"
type workersPerOS struct {
	Linux   int
	Windows int
}

// Helper to construct resize status command
func GetResizeStatusCommand(conf ExecConfig) []string {
	cmd := []string{"timeout", fmt.Sprintf("%d", conf.TimeoutSecs), conf.ResizePath, "status", "-baseport", fmt.Sprintf("%d", conf.BasePort)}
	if conf.RequireScriptVerification {
		cmd = append(cmd, "-secretfile", conf.SecretPath)
	}
	return cmd
}
