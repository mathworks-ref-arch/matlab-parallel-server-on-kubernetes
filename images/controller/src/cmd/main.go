// Package main runs the MJS in Kubernetes controller.
// Copyright 2024-2026 The MathWorks, Inc.
package main

import (
	"controller/internal/config"
	"controller/internal/controller"
	"controller/internal/jsonmarshal"
	"controller/internal/k8s"
	"controller/internal/k8swrapper"
	"controller/internal/logging"
	"controller/internal/profile"
	"controller/internal/request"
	"controller/internal/resize"
	"controller/internal/setup"
	"controller/internal/specs"
	"controller/internal/templatestore"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	conf := loadConfig()
	logger := createLogger(conf.LogFile, conf.LogLevel)
	defer logger.Close()

	client := getKubernetesClient(conf, logger)
	setUpCluster(conf, client, logger)
	controller := createController(conf, client, logger)

	// Catch SIGTERMs in a channel
	cancelChan := make(chan os.Signal, 1)
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)

	// Run controller loop until a SIGTERM is caught
	logger.Info("Starting MJS controller")
	ticker := time.NewTicker(time.Duration(conf.Period) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-cancelChan:
			logger.Debug("Stopping controller")
			return
		case <-ticker.C:
			controller.Run()
		}
	}
}

func errorAndExit(err error, logger *logging.Logger) {
	fmt.Println(err.Error())
	logger.Error("Fatal error", zap.Error(err))
	os.Exit(1)
}

// loadConfig reads the path to a config file from the command line arguments and reads in the config file
func loadConfig() *config.Config {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to config file")
	flag.Parse()
	if configFile == "" {
		errorAndExit(errors.New("must provide path to config file"), nil)
	}
	conf, err := config.LoadConfig(configFile)
	if err != nil {
		errorAndExit(err, nil)
	}
	return conf
}

// Create a logger
func createLogger(logFile string, logLevel int) *logging.Logger {
	logger, err := logging.NewLogger(logFile, logLevel)
	if err != nil {
		errorAndExit(err, nil)
	}
	return logger
}

// Load kubeconfig and create a client
func getKubernetesClient(conf *config.Config, logger *logging.Logger) k8s.Client {
	var kubeConfig *rest.Config
	var err error
	if conf.LocalDebugMode {
		kubeConfigFile := conf.KubeConfig
		if kubeConfigFile == "" {
			kubeConfigFile = filepath.Join(homedir.HomeDir(), ".kube", "config") // Use default kubeconfig location
		}
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		errorAndExit(err, logger)
	}

	// Create client
	k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		errorAndExit(fmt.Errorf("failed to create Kubernetes REST client: %v", err), logger)
	}
	client, err := k8swrapper.NewClientWrapper(k8swrapper.K8sConfig{
		Namespace:           conf.Network.Namespace,
		OwnerDeploymentName: conf.ResourceNames.Controller,
		TimeoutSecs:         conf.KubernetesTimeoutSecs,
	}, kubeConfig, k8sClient, logger)
	if err != nil {
		errorAndExit(err, logger)
	}
	return client
}

// Perform cluster setup tasks before running the controller loop
func setUpCluster(conf *config.Config, client k8s.Client, logger *logging.Logger) {
	profCreator := profile.New(profile.ProfileConfig{
		BasePort:                        conf.Network.BasePort,
		JobManagerName:                  conf.JobManagerName,
		JobManagerService:               conf.ResourceNames.JobManagerService,
		ParallelServerProxyPort:         conf.Network.ParallelServerProxyPort,
		ParallelServerProxyUseMutualTLS: conf.Network.ParallelServerProxyUseMutualTLS,
	}, logger)
	s := setup.New(setup.SetupConfig{
		JobManagerWaitPeriod:    conf.JobManagerWaitPeriod,
		LoadBalancerCheckPeriod: 2,
		PreserveSecrets:         conf.PreserveSecrets,
	}, conf.Network, conf.ResourceNames, client, certificate.New(), profCreator, jsonmarshal.New(), logger)

	setupSteps := []func() error{
		s.CheckAdminPassword,
		s.CheckLoadBalancer,
		s.CheckLDAPSecret,
		s.CreateOrLoadSharedSecret,
		s.CreateCertsForMetricsIfNeeded,
		s.CreateParallelServerProxySecretIfNeeded,
		s.WaitForJobManager,
		s.CreateProfile,
	}
	for _, step := range setupSteps {
		if err := step(); err != nil {
			errorAndExit(err, logger)
		}
	}
}

func createController(conf *config.Config, client k8s.Client, logger *logging.Logger) *controller.Controller {
	// The request getter talks to the job manager to get its desired worker count
	requestGetter := request.NewPodResizeRequestGetter(request.ExecConfig{
		BasePort:                  conf.Network.BasePort,
		JobManagerContainer:       conf.ResourceNames.JobManagerContainer,
		JobManagerLabel:           conf.ResourceNames.JobManagerLabel,
		JobManagerName:            conf.JobManagerName,
		RequireScriptVerification: conf.Network.RequireScriptVerification,
		ResizePath:                conf.ResizePath,
		SecretPath:                filepath.Join(conf.ResourceNames.SharedSecretDir, conf.ResourceNames.SharedSecretFile),
		TimeoutSecs:               conf.KubernetesTimeoutSecs - 5, // Use a slightly shorter timeout than the Kubernetes client so we don't end up with orphaned requests on the pod
	}, client, func(err error) { errorAndExit(err, logger) }, logger)

	// The template store keeps track of the latest worker pod template
	templateStore, err := templatestore.New(templatestore.StoreConfig{
		Namespace:             conf.Network.Namespace,
		PoolProxyTemplateName: conf.ResourceNames.PoolProxyTemplate,
		UsePoolProxy:          conf.Network.UsePoolProxy,
		WorkerTemplateName:    conf.ResourceNames.WorkerTemplate,
		TemplateFile:          conf.ResourceNames.TemplateFile,
	}, client, logger)
	if err != nil {
		errorAndExit(err, logger)
	}

	// The spec factory converts templates into pod specs for a given worker
	specFactory := specs.NewFactory(conf.AnnotationKeys, templateStore, logger)

	// The resize scales worker pods up or down
	resizer := resize.NewK8sResizer(resize.ResizeConfig{
		ProxyCertFile:  conf.ResourceNames.CertificateFile,
		WorkerLabel:    conf.ResourceNames.WorkerLabel,
		PoolProxyLabel: conf.ResourceNames.PoolProxyLabel,
		AnnotationKeys: conf.AnnotationKeys,
	}, client, specFactory, templateStore, certificate.New(), logger)

	// The controller reconciles the desired worker count with the actual worker count
	return controller.New(conf.IdleStop, conf.MinWorkers, requestGetter, resizer, logger)
}
