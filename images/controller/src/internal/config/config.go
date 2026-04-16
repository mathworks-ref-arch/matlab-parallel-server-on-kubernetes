// Define configurable controller settings and enable them to be loaded from a JSON file
// Copyright 2024-2026 The MathWorks, Inc.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config contains configurable controller settings
type Config struct {
	ControllerConfig
	AnnotationKeys AnnotationKeys
	Network        NetworkConfig
	ResourceNames  ResourceNames
}

// Configuration for controller behaviour
type ControllerConfig struct {
	IdleStop              int    // Time in seconds after which an idle worker can be stopped
	LocalDebugMode        bool   // Set to true if running the controller outside of the Kubernetes cluster
	LogFile               string // File to write controller logs to. If empty, the controller logs to stdout
	LogLevel              int    // PCT log level
	JobManagerName        string // Name of the MJS job manager
	JobManagerWaitPeriod  int    // Retry period when waiting for the job manager pod to be ready
	KubeConfig            string // Kubeconfig file to use if running in local debug mode
	KubernetesTimeoutSecs int    // Timeout for Kubernetes API operations
	MinWorkers            int    // Minimum number of worker pods to maintain
	Period                int    // Period for the controller loop
	PreserveSecrets       bool   // If true, secrets created during controller setup will not be deleted when the controller is stopped
	ResizePath            string // Path to the "resize" script on the job manager pod
}

// Configuration for cluster networking
type NetworkConfig struct {
	BasePort                         int    // MJS base port
	ClusterDomain                    string // Domain of the Kubernetes cluster (e.g. "cluster.local")
	ClusterHost                      string // Optional custom hostname to use in the cluster profile
	InternalClientsOnly              bool   // Set to true if the cluster is configured such that MATLAB clients must run inside the Kubernetes cluster
	JobManagerPort                   int    // Port used for the job manager service
	LookupPort                       int    // Port used for the lookup service
	MaxWorkers                       int    // Maximum workers this MJS cluster supports
	Namespace                        string // Kubernetes namespace we are running in
	OpenMetricsPortOutsideKubernetes bool   // Whether to expose job manager metrics outside Kubernetes
	ParallelServerProxyPort          int    // Port to use for the parallel server proxy
	ParallelServerProxyUseMutualTLS  bool   // Set to true if the parallel server proxy uses mutual TLS
	PoolProxyBasePort                int    // Base port to use for pool proxies
	RequireClientCertificate         bool   // Set to true if MJS requires MATLAB clients to have a certificate
	RequireLdapCert                  bool   // Set to true if MJS is configured to use secure LDAP
	RequireScriptVerification        bool   // Set to true if MJS is configured to require verification of admin operations
	SecurityLevel                    int    // MJS security level
	WorkersPerPoolProxy              int    // Number of workers that can be served by a single pool proxy
	UsePoolProxy                     bool   // Set to true if this cluster uses pool proxies to proxy client-worker traffic
	UseParallelServerProxy           bool   // Set to true if this cluster uses a parallel server proxy (supported for post-R2026a only)
	UseSecureCommunication           bool   // Set to true if MJS is configured to use secure communication between services
	UseSecureMetrics                 bool   // Set to true if the job manager is configured to use an HTTPs metrics server
}

// Names of Kubernetes resources that the controller interacts with
type ResourceNames struct {
	AdminPasswordSecret       string // Name of secret containing the MJS admin password
	AdminPasswordKey          string // Key containing the admin password in the secret
	CertificateFile           string // Name of mounted certificate file
	ClientMetricsSecret       string // Name of secret containing client metrics certificates
	ClientMetricsCertFile     string // Name of certificate file in client metrics secret
	ClientMetricsKeyFile      string // Name of private key file in client metrics secret
	Controller                string // Name of the controller deployment
	JobManagerContainer       string // Name of the container in which the job manager runs
	JobManagerLabel           string // Label that can be used to identify the job manager pod
	JobManagerService         string // Name of the job manager service
	LdapSecret                string // Name of secret containing LDAP certificates
	LdapCertFile              string // Name of certificate file in LDAP secret
	LoadBalancer              string // Name of the load balancer service
	MetricsSecret             string // Name of secret containing certificates for the job manager metrics server
	MetricsCaCertFile         string // Name of CA certificate file in metrics secret
	MetricsCertFile           string // Name of server certificate file in metrics secret
	MetricsKeyFile            string // Name of server private key file in metrics secret
	ParallelServerProxySecret string // Name of secret containing parallel server proxy certificate
	PoolProxyLabel            string // Label that can be used to identify pool proxy deployments
	PoolProxyTemplate         string // Name of the pool proxy pod template
	ProfileKey                string // Key of the cluster profile file within a profile secret
	ProfileSecret             string // Name of the profile secret used for the MJS cluster
	ProfileSecretPre26a       string // Name of the additional profile secret generated for a backwards-compatible cluster that supports releases older than R2026a
	SharedSecret              string // Name of the MJS shared secret
	SharedSecretDir           string // Directory in which the MJS shared secret is mounted on the job manager pod
	SharedSecretFile          string // Name of the MJS shared secret file
	TemplateFile              string // Name of pod template file within a config map
	WorkerLabel               string // Label that can be used to identify worker deployments
	WorkerTemplate            string // Name of the worker pod template
}

// Keys for annotations used to extract settings from pod specs
type AnnotationKeys struct {
	CertVolume             string
	LogDir                 string
	PoolProxyBasePort      string
	PoolProxyID            string
	PoolProxyPrefix        string
	SecretName             string
	TemplateChecksum       string
	UsePoolProxy           string
	UseSecureCommunication string
	WorkerDomain           string
	WorkerName             string
	WorkerID               string
	WorkerPrefix           string
	WorkersPerPoolProxy    string
}

// LoadConfig reads a Config object from a JSON file
func LoadConfig(configFile string) (*Config, error) {
	var err error
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %v", err)
	}
	defer func() {
		cerr := file.Close()
		if cerr != nil {
			err = cerr
		}
	}()

	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()

	var config Config
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from config file: %v", err)
	}
	return &config, err
}
