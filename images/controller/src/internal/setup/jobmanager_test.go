// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/config"
	"controller/internal/setup"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaitForJobManager(t *testing.T) {
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{
		JobManagerWaitPeriod: 0,
	}, config.NetworkConfig{})

	// First return false, then true
	mocks.client.EXPECT().IsPodReady(testResources.JobManagerLabel, testResources.JobManagerContainer).Once().Return(false, nil)
	mocks.client.EXPECT().IsPodReady(testResources.JobManagerLabel, testResources.JobManagerContainer).Once().Return(true, nil)

	err := s.WaitForJobManager()
	require.NoError(t, err)
}

func TestWaitForJobManagerError(t *testing.T) {
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{
		JobManagerWaitPeriod: 0,
	}, config.NetworkConfig{})

	// Configure client to error
	clientErr := errors.New("failed to check pod")
	mocks.client.EXPECT().IsPodReady(testResources.JobManagerLabel, testResources.JobManagerContainer).Once().Return(false, clientErr)

	err := s.WaitForJobManager()
	require.Error(t, err, "Expected error when client errors")
	verifyErrorContainsClientError(t, err, clientErr)
}
