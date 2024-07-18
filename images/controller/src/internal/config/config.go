// Package config defines configurable controller settings and enables them to be loaded from a JSON file
// Copyright 2024 The MathWorks, Inc.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Config contains configurable controller settings
type Config struct {
	ControllerLogfile             string
	BasePort                      int
	CertFileName                  string
	CheckpointBase                string
	CheckpointPVC                 string
	ClusterHost                   string
	DeploymentName                string
	EnableServiceLinks            bool
	ExtraWorkerEnvironment        map[string]string
	JobManagerUID                 string
	IdleStop                      int
	InternalClientsOnly           bool
	JobManagerName                string
	JobManagerCPULimit            string
	JobManagerCPURequest          string
	JobManagerImage               string
	JobManagerImagePullPolicy     string
	JobManagerMemoryLimit         string
	JobManagerMemoryRequest       string
	JobManagerGroupID             int64
	JobManagerUserID              int64
	KubeConfig                    string
	LivenessProbeFailureThreshold int32
	LivenessProbePeriod           int32
	LivenessProbeTimeout          int32
	LoadBalancerName              string
	LocalDebugMode                bool
	LogBase                       string
	LogLevel                      int
	LogPVC                        string
	MatlabRoot                    string
	MatlabPVC                     string
	MaxWorkers                    int
	MinWorkers                    int
	MJSDefConfigMap               string
	MJSDefDir                     string
	Namespace                     string
	NetworkLicenseManager         string
	Period                        int
	PortsPerWorker                int
	PoolProxyBasePort             int
	PoolProxyCPULimit             string
	PoolProxyCPURequest           string
	PoolProxyImage                string
	PoolProxyImagePullPolicy      string
	PoolProxyMemoryLimit          string
	PoolProxyMemoryRequest        string
	ReadyFile                     string
	ResizePath                    string
	RequireClientCertificate      bool
	RequireScriptVerification     bool
	SecretDir                     string
	SecretFileName                string
	SecurityLevel                 int
	StartupProbeFailureThreshold  int32
	StartupProbeInitialDelay      int32
	StartupProbePeriod            int32
	StopWorkerGracePeriod         int64
	WorkerCPURequest              string
	WorkerCPULimit                string
	WorkerImage                   string
	WorkerImagePullPolicy         string
	WorkerLogPVC                  string
	WorkerMemoryRequest           string
	WorkerMemoryLimit             string
	WorkerPassword                string
	WorkersPerPoolProxy           int
	WorkerUsername                string
	UseSecureCommunication        bool
}

// LoadConfig reads a Config object from a JSON file
func LoadConfig(configFile string) (*Config, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}
	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from config file: %v", err)
	}
	return &config, nil
}

// RequiresSecret returns true if the cluster configuration requires a shared secret
func (c *Config) RequiresSecret() bool {
	return c.UseSecureCommunication || c.RequireClientCertificate || c.RequireScriptVerification
}

// UsePoolProxy returns true if we should install pool proxies
func (c *Config) UsePoolProxy() bool {
	return !c.InternalClientsOnly
}
