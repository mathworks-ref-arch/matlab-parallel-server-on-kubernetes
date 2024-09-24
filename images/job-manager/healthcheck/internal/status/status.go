// Copyright 2023-2024 The MathWorks, Inc.
package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

type StatusGetter interface {
	GetJobManagers() ([]JobManagerStatus, error)
	GetWorkerGroupStatus() (string, error)
}

type JobManagerStatus struct {
	Name   string
	Status string
}

type WorkerGroupStatus struct {
	Status string
}

type nodeStatus struct {
	JobManagers []JobManagerStatus
	WorkerGroup WorkerGroupStatus
}

type nodeStatusFunc = func(context.Context, string, ...string) ([]byte, error)

type NodeStatusRunner struct {
	nodeStatusPath string
	timeout        time.Duration
	basePort       int

	// Allow the nodestatus function call to be replaced with a mock
	runNodeStatusFunc nodeStatusFunc
}

func NewNodeStatusRunner(matlabRoot string, timeoutSeconds, basePort int) *NodeStatusRunner {
	n := NodeStatusRunner{
		nodeStatusPath: filepath.Join(filepath.FromSlash(matlabRoot), "toolbox", "parallel", "bin", "nodestatus"),
		timeout:        time.Duration(timeoutSeconds * int(time.Second)),
		basePort:       basePort,
		runNodeStatusFunc: func(ctx context.Context, path string, arg ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, path, arg...)
			return cmd.Output()
		},
	}
	return &n
}

// Get a list of job managers and their statuses
func (n *NodeStatusRunner) GetJobManagers() ([]JobManagerStatus, error) {
	status, err := n.getNodeStatus()
	if err != nil {
		return []JobManagerStatus{}, err
	}
	return status.JobManagers, nil
}

// Get worker group status
func (n *NodeStatusRunner) GetWorkerGroupStatus() (string, error) {
	status, err := n.getNodeStatus()
	if err != nil {
		return "", err
	}
	return status.WorkerGroup.Status, nil
}

func (n *NodeStatusRunner) getNodeStatus() (*nodeStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	// Pass the -baseport argument to nodestatus if basePort is set
	args := []string{"-json"}
	if n.basePort != -1 {
		args = append(args, "-baseport", fmt.Sprintf("%d", n.basePort))
	}
	output, err := n.runNodeStatusFunc(ctx, n.nodeStatusPath, args...)

	// Check if the command timed out
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("error: nodestatus command failed to complete within %.0f seconds", n.timeout.Seconds())
	}

	// Check if nodestatus errored
	if err != nil {
		errMsg := fmt.Sprintf("error executing nodestatus: %v", err)

		// Try to get stderr from nodestatus
		cmdOut := cmdOutput{}
		unmarshalOutputErr := json.Unmarshal(output, &cmdOut)
		if unmarshalOutputErr == nil {
			errMsg = errMsg + "\n" + cmdOut.Error
		}
		return nil, fmt.Errorf(errMsg)
	}

	// Parse the raw output
	status := nodeStatus{}
	err = json.Unmarshal(output, &status)
	if err != nil {
		return nil, fmt.Errorf("error parsing the output of nodestatus: %v", err)
	}
	return &status, nil
}

type cmdOutput struct {
	Error string
}
