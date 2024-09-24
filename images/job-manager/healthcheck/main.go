// Copyright 2023 The MathWorks, Inc.
//nolint
package main

import (
	"flag"
	"fmt"
	"mjshealthcheck/internal/healthcheck"
	"mjshealthcheck/internal/status"
	"os"
)

const (
	exitUnhealthy = 1
	exitError     = 2
)

// Tool for performing an MJS job manager healthcheck.
// Exit code 0 = the job manager is healthy
// Exit code 1 = the job manager is unhealthy or was not found
// Exit code 2 = an error occurred while performing the healthcheck
func main() {
	inputOpts := parseFlags()
	healthchecker := healthcheck.NewHealthChecker(status.NewNodeStatusRunner(inputOpts.matlabRoot, inputOpts.timeout, inputOpts.basePort))

	var healthy bool
	var msg string
	var err error
	if inputOpts.isWorkerCheck {
		healthy, msg, err = healthchecker.DoWorkerGroupHealthcheck()
	} else {
		healthy, msg, err = healthchecker.DoJobManagerHealthcheck(inputOpts.jobManagerName)
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(exitError)
	}
	if !healthy {
		fmt.Println(msg)
		os.Exit(exitUnhealthy)
	}
}

type opts struct {
	matlabRoot     string
	jobManagerName string
	timeout        int
	basePort       int
	isWorkerCheck  bool
}

func parseFlags() *opts {
	inputOpts := opts{}
	// By default, assume we are running from the directory of the executable (matlab/toolbox/parallel/bin/${ARCH})
	flag.StringVar(&inputOpts.matlabRoot, "matlabroot", "../../../..", "Path to MATLAB root")
	flag.StringVar(&inputOpts.jobManagerName, "jobmanager", "", "Name of the job manager on which to perform the healthcheck if multiple job managers are running")
	flag.BoolVar(&inputOpts.isWorkerCheck, "worker", false, "Flag to perform a healthcheck on the worker group instead of a job manager")
	flag.IntVar(&inputOpts.timeout, "timeout", 60, "Timeout in seconds for running the nodestatus command")
	flag.IntVar(&inputOpts.basePort, "baseport", -1, "The base port that the MJS service is using")
	flag.Parse()

	// We cannot do both a worker healthcheck and a job manager check
	if inputOpts.isWorkerCheck && inputOpts.jobManagerName != "" {
		fmt.Println("error: healthcheck can only be performed on a job manager or a worker, not both. Provide only the -jobmanager flag or the -worker flag.")
		os.Exit(exitError)
	}

	return &inputOpts
}
