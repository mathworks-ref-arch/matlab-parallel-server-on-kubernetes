// Package controller provides code for setting up and automatically rescaling a cluster
// Copyright 2024 The MathWorks, Inc.
package controller

import (
	"controller/internal/config"
	"controller/internal/k8s"
	"controller/internal/logging"
	"controller/internal/rescaler"
	"controller/internal/specs"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"github.com/mathworks/mjssetup/pkg/profile"
	"go.uber.org/zap"
)

// Controller sets up and periodically rescales a cluster
type Controller struct {
	config            *config.Config
	logger            *logging.Logger
	client            k8s.Client
	specFactory       *specs.SpecFactory
	waitForJobManager func() error      // Function to wait until the job manager is ready
	period            time.Duration     // Interval between each rescaling operation
	rescaler          rescaler.Rescaler // Interface to perform the rescaling
	stopChan          chan bool         // Channel to capture stop signals
}

// NewController constructs a Controller from a given config struct
func NewController(conf *config.Config, logger *logging.Logger) (*Controller, error) {
	logger.Debug("Creating controller", zap.Any("config", conf))

	// Create Kubernetes client
	client, err := k8s.NewClient(conf, logger)
	if err != nil {
		return nil, err
	}

	// Get the UID of the deployment in which we are running; we use this to tag all created resources so they are cleaned up by the Kubernetes garbage collector if the controller is removed
	uid, err := client.GetControllerDeploymentUID()
	if err != nil {
		return nil, err
	}

	// Create the MJS rescaler
	rescaler, err := rescaler.NewMJSRescaler(conf, uid, logger)
	if err != nil {
		return nil, err
	}

	controller := &Controller{
		config:      conf,
		logger:      logger,
		specFactory: specs.NewSpecFactory(conf, uid),
		period:      time.Duration(conf.Period) * time.Second,
		rescaler:    rescaler,
		stopChan:    make(chan bool),
		client:      client,
		waitForJobManager: func() error {
			return waitForJobManager(client, conf.StartupProbePeriod)
		},
	}

	err = controller.setup()
	if err != nil {
		return nil, err
	}
	return controller, nil
}

// Run autoscaling periodically until a stop signal is received
func (c *Controller) Run() {
	ticker := time.NewTicker(c.period)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopChan:
			c.logger.Debug("Stopping controller")
			return
		case <-ticker.C:
			c.rescaler.Rescale()
		}
	}
}

// Stop can be called asynchronously to stop the controller from running
func (c *Controller) Stop() {
	c.stopChan <- true
}

// Perform initial MJS cluster setup
func (c *Controller) setup() error {
	err := c.checkRequiredResources()
	if err != nil {
		return err
	}
	sharedSecret, err := c.createMJSSecrets()
	if err != nil {
		return err
	}
	err = c.createJobManager()
	if err != nil {
		return err
	}
	return c.createProfile(sharedSecret)
}

// Check that the prerequiste Kubernetes resources exist
func (c *Controller) checkRequiredResources() error {
	checksToRun := []func() error{
		c.checkAdminPassword,
		c.checkLoadBalancer,
		c.checkLDAPSecret,
	}
	for _, checkFunc := range checksToRun {
		err := checkFunc()
		if err != nil {
			return err
		}
	}
	return nil
}

// Check that the load balancer service exists and exposes the correct ports
func (c *Controller) checkLoadBalancer() error {
	if c.config.InternalClientsOnly {
		// There is no load balancer for internal-only mode
		return nil
	}

	svc, exists, err := c.client.ServiceExists(c.config.LoadBalancerName)
	if err != nil {
		return err
	}

	// Compute the ports we expect this service to expose
	requiredPorts := []int{c.config.BasePort + 6, c.config.BasePort + 9}
	maxPoolProxies := int(math.Ceil(float64(c.config.MaxWorkers) / float64(c.config.WorkersPerPoolProxy)))
	for i := 0; i < maxPoolProxies; i++ {
		requiredPorts = append(requiredPorts, c.config.PoolProxyBasePort+i)
	}

	// Error if the service does not exist
	if !exists {
		portPairs := ""
		for idx, p := range requiredPorts {
			if idx > 0 {
				portPairs += ","
			}
			portPairs += fmt.Sprintf("%d:%d", p, p)
		}
		exampleCmd := fmt.Sprintf("kubectl create service loadbalancer %s --namespace %s --tcp %s", c.config.LoadBalancerName, c.config.Namespace, portPairs)
		return fmt.Errorf(`error: Load balancer service "%s" does not exist in namespace "%s". Create a load balancer service configured for MATLAB Job Scheduler with command: "%s"`, c.config.LoadBalancerName, c.config.Namespace, exampleCmd)
	}

	// If the service exists, check that all ports are exposed correctly
	exposedPorts := map[int]bool{}
	for _, p := range svc.Spec.Ports {
		port := int(p.Port)
		targetPort := p.TargetPort.IntValue()
		if port != targetPort {
			return fmt.Errorf(`error: Target port %d does not match service port %d in specification for load balancer service "%s". Modify the service specification so that all target ports match service ports`, targetPort, port, c.config.LoadBalancerName)
		}
		exposedPorts[port] = true
	}
	foundMissing := false
	missingPorts := ""
	for _, p := range requiredPorts {
		if !exposedPorts[p] {
			if foundMissing {
				missingPorts += ", "
			}
			missingPorts += fmt.Sprintf("%d", p)
			foundMissing = true
		}
	}
	if foundMissing {
		return fmt.Errorf(`error: Load balancer service "%s" does not expose all ports required by MATLAB Job Scheduler. Missing ports: %s. Modify the service specification to expose all required ports`, c.config.LoadBalancerName, missingPorts)
	}
	return nil
}

// Check that the administrator password exists, if needed
func (c *Controller) checkAdminPassword() error {
	if c.config.SecurityLevel < 2 {
		// No admin password required for security level 1 or below
		return nil
	}
	adminSecretName := specs.AdminPasswordSecretName
	passwordKey := specs.AdminPasswordKey
	secret, adminPasswordExists, err := c.client.SecretExists(adminSecretName)
	if err != nil {
		return err
	}
	createSecretInstruction := fmt.Sprintf(`To start an MJS cluster at security level %d, create an administrator password secret with command "kubectl create secret generic %s --from-literal=%s=<password> --namespace %s", replacing "<password>" with a password of your choice.`, c.config.SecurityLevel, adminSecretName, passwordKey, c.config.Namespace)
	if !adminPasswordExists {
		return fmt.Errorf(`error: Administrator password secret "%s" does not exist in namespace "%s". %s`, adminSecretName, c.config.Namespace, createSecretInstruction)
	}

	// Check that the secret contains the password key
	if _, ok := secret.Data[passwordKey]; !ok {
		return fmt.Errorf(`error: Administrator password secret "%s" does not contain the key "%s". %s`, specs.AdminPasswordSecretName, passwordKey, createSecretInstruction)
	}
	return nil
}

// Check that the LDAP certificate secret exists, if needed
func (c *Controller) checkLDAPSecret() error {
	if c.config.LDAPCertPath == "" {
		// No need for LDAP certificate
		return nil
	}
	ldapSecretName := specs.LDAPSecretName
	certFile := c.config.LDAPCertFile()
	secret, exists, err := c.client.SecretExists(ldapSecretName)
	if err != nil {
		return err
	}
	createSecretInstruction := fmt.Sprintf(`To start an MJS using a secure LDAP server to authenticate user credentials, create an LDAP certificate secret with command "kubectl create secret generic %s --from-file=%s=<path> --namespace %s", replacing "<path>" with the path to the SSL certificate for your LDAP server.`, ldapSecretName, certFile, c.config.Namespace)
	if !exists {
		return fmt.Errorf(`error: LDAP certificate secret "%s" does not exist in namespace "%s". %s`, ldapSecretName, c.config.Namespace, createSecretInstruction)
	}

	// Check that the secret contains the expected filename
	if _, ok := secret.Data[certFile]; !ok {
		return fmt.Errorf(`error: LDAP certificate secret "%s" does not contain the file "%s". %s`, specs.AdminPasswordSecretName, certFile, createSecretInstruction)
	}
	return nil
}

// Create MJS secrets
func (c *Controller) createMJSSecrets() (*certificate.SharedSecret, error) {
	var sharedSecret *certificate.SharedSecret
	var err error
	if c.config.RequiresSecret() {
		sharedSecret, err = c.createSharedSecret()
		if err != nil {
			return nil, err
		}
	}
	return sharedSecret, nil
}

// Create shared secret and certificate for MJS and return the shared secret
func (c *Controller) createSharedSecret() (*certificate.SharedSecret, error) {
	secret, alreadyExists, err := c.getExistingSharedSecret()
	if err != nil {
		return nil, fmt.Errorf("error checking for shared secret: %v", err)
	}
	if alreadyExists {
		return secret, err
	}

	// Generate the shared secret
	certCreator := certificate.New()
	secret, err = certCreator.CreateSharedSecret()
	if err != nil {
		return nil, err
	}
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("error marshalling shared secret: %v", err)
	}

	// Get spec for Kubernetes secret
	secretSpec := c.specFactory.GetSecretSpec(specs.SharedSecretName)
	secretSpec.Data[c.config.SecretFileName] = secretBytes

	// Generate a certificate if needed
	if c.config.RequireClientCertificate {
		cert, err := certCreator.GenerateCertificate(secret)
		if err != nil {
			return nil, err
		}
		certBytes, err := json.Marshal(cert)
		if err != nil {
			return nil, fmt.Errorf("error marshalling certificate: %v", err)
		}
		secretSpec.Data[c.config.CertFileName] = certBytes
	}

	// Create the Kubernetes secret
	_, err = c.client.CreateSecret(secretSpec)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes secret for MJS shared secret: %v", err)
	}
	return secret, nil
}

// Create a deployment for the MJS job manager; return when the pod is ready
func (c *Controller) createJobManager() error {
	// Check whether the deployment already exists; this can occur if the controller container has restarted
	alreadyExists, err := c.client.DeploymentExists(specs.JobManagerHostname)
	if err != nil {
		return fmt.Errorf("error checking for job manager deployment: %v", err)
	}
	if alreadyExists {
		c.logger.Info("found existing job manager deployment", zap.String("name", specs.JobManagerHostname))
		return nil
	}

	// Create deployment
	deploymentSpec := c.specFactory.GetJobManagerDeploymentSpec()
	deployment, err := c.client.CreateDeployment(deploymentSpec)
	if err != nil {
		return fmt.Errorf("error creating job manager deployment: %v", err)
	}
	c.logger.Info("created MJS job manager deployment", zap.String("name", deployment.Name))

	// Wait for the pod to be ready before returning
	c.logger.Info("waiting for job manager pod to be ready")
	err = c.waitForJobManager()
	if err != nil {
		return err
	}
	c.logger.Info("found ready job manager pod")
	return nil
}

// Profile secret names
const (
	profileSecretName = "mjs-cluster-profile"
	profileKey        = "profile"
)

// Create the cluster profile
func (c *Controller) createProfile(sharedSecret *certificate.SharedSecret) error {
	_, alreadyExists, err := c.client.SecretExists(profileSecretName)
	if err != nil {
		return fmt.Errorf("error checking for cluster profile secret: %v", err)
	}
	if alreadyExists {
		c.logger.Info("found existing cluster profile password secret", zap.String("name", profileSecretName))
		return nil
	}

	// Get MJS hostname
	var clusterHost = c.config.ClusterHost
	if clusterHost == "" {
		if c.config.InternalClientsOnly {
			// Use the job manager hostname if all clients are inside the Kubernetes cluster
			clusterHost = c.specFactory.GetServiceHostname(specs.JobManagerHostname)
		} else {
			// Extract the hostname from the load balancer
			var err error
			clusterHost, err = c.getExternalAddress()
			if err != nil {
				return err
			}
		}
	}

	// Generate a certificate for the client if needed
	var cert *certificate.Certificate
	if c.config.RequireClientCertificate {
		cert, err = certificate.New().GenerateCertificate(sharedSecret)
		if err != nil {
			return fmt.Errorf("error generating certificate for cluster profile: %v", err)
		}
	}

	// Create the profile
	profile := profile.CreateProfile(c.config.JobManagerName, clusterHost, cert)
	profBytes, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling cluster profile into bytes: %v", err)
	}

	// Create Kubernetes secret for profile
	secret := c.specFactory.GetSecretSpec(profileSecretName)
	secret.Data[profileKey] = profBytes
	_, err = c.client.CreateSecret(secret)
	if err != nil {
		return fmt.Errorf("error creating Kubernetes secret for MJS cluster profile: %v", err)
	}
	c.logger.Info("created MJS cluster profile secret", zap.String("name", secret.Name))

	return nil
}

// Get the external address of MJS
func (c *Controller) getExternalAddress() (string, error) {
	addressFound := false
	retryPeriod := time.Duration(2 * time.Second)
	c.logger.Info("waiting for LoadBalancer service to have external hostname", zap.String("serviceName", c.config.LoadBalancerName))
	address := ""
	for !addressFound {
		loadBalancer, err := c.client.GetLoadBalancer()
		if err != nil {
			return "", err
		}
		for _, ingress := range loadBalancer.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				address = ingress.IP
				addressFound = true
			} else if ingress.Hostname != "" {
				address = ingress.Hostname
				addressFound = true
			}
		}
		time.Sleep(retryPeriod)
	}
	c.logger.Info("found LoadBalancer external hostname", zap.String("hostname", address))

	// Append the base port
	address = fmt.Sprintf("%s:%d", address, c.config.BasePort)
	return address, nil
}

// Extract a shared secret from a Kubernetes secret if one already exists
func (c *Controller) getExistingSharedSecret() (*certificate.SharedSecret, bool, error) {
	k8sSecret, alreadyExists, err := c.client.SecretExists(specs.SharedSecretName)
	if err != nil {
		return nil, false, fmt.Errorf("error checking for shared secret: %v", err)
	}
	if !alreadyExists {
		return nil, false, nil
	}

	c.logger.Info("found existing shared secret", zap.String("name", specs.SharedSecretName))
	secretData, hasSecret := k8sSecret.Data[c.config.SecretFileName]
	if !hasSecret {
		return nil, false, fmt.Errorf("secret file '%s' not found in Kubernetes Secret '%s'", c.config.SecretFileName, k8sSecret.Name)
	}
	secret, err := certificate.New().LoadSharedSecret(secretData)
	if err != nil {
		return nil, false, fmt.Errorf("error extracting shared secret from Kubernetes Secret '%s': %v", k8sSecret.Name, err)
	}
	return secret, true, nil
}

// Wait for the job manager to be ready
func waitForJobManager(client k8s.Client, retryPeriodSeconds int32) error {
	retryPeriod := time.Duration(retryPeriodSeconds) * time.Second
	for {
		isReady, err := client.IsJobManagerReady()
		if err != nil {
			return err
		}
		if isReady {
			break
		}
		time.Sleep(retryPeriod)
	}
	return nil
}
