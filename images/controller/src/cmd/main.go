// Package main runs the MJS in Kubernetes controller.
// Copyright 2024 The MathWorks, Inc.
package main

import (
	"controller/internal/config"
	"controller/internal/controller"
	"controller/internal/logging"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create logger
	logger, loggerErr := logging.NewLogger(config.ControllerLogfile, config.LogLevel)
	if loggerErr != nil {
		fmt.Printf("Error creating logger: %v\n", loggerErr)
		os.Exit(1)
	}
	defer logger.Close()

	// Catch SIGTERMs in a channel
	cancelChan := make(chan os.Signal, 1)
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)

	// Run controller
	logger.Info("Starting MJS controller")
	scaler, err := controller.NewController(config, logger)
	if err != nil {
		fmt.Println(err)
		logger.Error("Error creating controller", zap.Any("error", err))
		os.Exit(1)
	}
	go scaler.Run()

	// Block until a cancellation is received
	sig := <-cancelChan
	logger.Info("Caught signal; shutting down", zap.Any("sig", sig))
	scaler.Stop()
}

// loadConfig reads the path to a config file from the command line arguments and reads in the config file
func loadConfig() (*config.Config, error) {
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to config file")
	flag.Parse()
	if configFile == "" {
		return nil, errors.New("must provide path to config file")
	}
	return config.LoadConfig(configFile)
}
