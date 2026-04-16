// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/config"
	"controller/internal/setup"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Test the case where the load balancer exists and has all required ports
func TestCheckLoadBalancerPositive(t *testing.T) {
	networkConf := config.NetworkConfig{
		BasePort:                27350,
		UsePoolProxy:            true,
		UseParallelServerProxy:  true,
		ParallelServerProxyPort: 1080,
		PoolProxyBasePort:       30000,
		WorkersPerPoolProxy:     4,
		MaxWorkers:              8, // Expect 2 pool proxies
		JobManagerPort:          4000,
		LookupPort:              4001,
	}

	svc := &corev1.Service{}
	addPortToService(svc, networkConf.ParallelServerProxyPort)
	addPortToService(svc, networkConf.LookupPort)
	addPortToService(svc, networkConf.JobManagerPort)
	addPortToService(svc, networkConf.PoolProxyBasePort)
	addPortToService(svc, networkConf.PoolProxyBasePort+1)

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mocks.client.EXPECT().ServiceExists(testResources.LoadBalancer).Return(svc, true, nil).Once()

	err := s.CheckLoadBalancer()
	assert.NoError(t, err, "CheckLoadBalancer should pass when all required ports are present")
}

// CheckLoadBalancer should be a no-op if we set InternalClientsOnly=true
func TestCheckLoadBalancerInternalClientOnly(t *testing.T) {
	networkConf := config.NetworkConfig{
		InternalClientsOnly: true,
	}
	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	err := s.CheckLoadBalancer() // Should be a no-op
	assert.NoError(t, err, "CheckLoadBalancer should pass when all required ports are present")
}

// CheckLoadBalancer should fail if the client doesn't find the load balancer
func TestCheckLoadBalancerMissingService(t *testing.T) {
	networkConf := config.NetworkConfig{
		UsePoolProxy:        true,
		PoolProxyBasePort:   20000,
		BasePort:            1000,
		MaxWorkers:          10,
		WorkersPerPoolProxy: 10,
		LookupPort:          3000,
		JobManagerPort:      4000,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mocks.client.EXPECT().ServiceExists(testResources.LoadBalancer).Return(nil, false, nil).Once()

	err := s.CheckLoadBalancer()
	require.Error(t, err, "Expected error when load balancer is missing")
	assert.Contains(t, err.Error(), testResources.LoadBalancer, "Error should contain name of missing load balancer")

	expectedPorts := []int{
		networkConf.LookupPort,
		networkConf.JobManagerPort,
		networkConf.PoolProxyBasePort,
	}
	for _, p := range expectedPorts {
		verifyErrorContainsPort(t, err, p)
	}
}

// CheckLoadBalancer should fail if the client errors trying to find the load balancer
func TestCheckLoadBalancerClientError(t *testing.T) {
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, config.NetworkConfig{})
	clientErr := expectCheckServiceError(mocks.client, testResources.LoadBalancer)

	err := s.CheckLoadBalancer()
	require.Error(t, err, "Expected error when load balancer is missing")
	verifyErrorContainsClientError(t, err, clientErr)
}

// Verify that we get an error when the load balancer has a mismatched port/targetport
func TestCheckLoadBalancerPortMismatch(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:  true,
		ParallelServerProxyPort: 1080,
	}

	// Misconfigure a port such that TargetPort != Port
	svc := &corev1.Service{}
	addPortToService(svc, networkConf.ParallelServerProxyPort)
	badPort := svc.Spec.Ports[0].Port
	badTarget := badPort - 1
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(int(badTarget))

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mocks.client.EXPECT().ServiceExists(testResources.LoadBalancer).Return(svc, true, nil).Once()

	err := s.CheckLoadBalancer()
	require.Error(t, err, "Expect error when a target port does not match a port")
	verifyErrorContainsPort(t, err, int(badPort))
	verifyErrorContainsPort(t, err, int(badTarget))
}

// Verify that we get an error when the load balancer is missing ports in the pool proxy case
func TestCheckLoadBalancerMissingPortsPoolProxy(t *testing.T) {
	networkConf := config.NetworkConfig{
		UsePoolProxy:        true,
		BasePort:            27350,
		PoolProxyBasePort:   30000,
		WorkersPerPoolProxy: 5,
		MaxWorkers:          10, // Expect 2 pool proxies
		LookupPort:          5000,
		JobManagerPort:      5002,
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	svc := &corev1.Service{} // Service with no ports
	mocks.client.EXPECT().ServiceExists(testResources.LoadBalancer).Return(svc, true, nil).Once()

	err := s.CheckLoadBalancer()
	require.Error(t, err, "Expect error when load balancer does not expose MJS ports")
	expectedPorts := []int{
		networkConf.LookupPort,
		networkConf.JobManagerPort,
		networkConf.PoolProxyBasePort,
		networkConf.PoolProxyBasePort + 1,
	}
	for _, p := range expectedPorts {
		verifyErrorContainsPort(t, err, p)
	}
}

// Verify that we get an error when the load balancer is missing a required port in the case where we use the parallel server proxy
func TestCheckLoadBalancerMissingPortParallelServerProxy(t *testing.T) {
	networkConf := config.NetworkConfig{
		UsePoolProxy:            false,
		UseParallelServerProxy:  true,
		ParallelServerProxyPort: 1080,
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	svc := &corev1.Service{} // Service with no ports
	mocks.client.EXPECT().ServiceExists(testResources.LoadBalancer).Return(svc, true, nil).Once()

	err := s.CheckLoadBalancer()
	require.Error(t, err, "Expect error when load balancer does not expose MJS ports")
	verifyErrorContainsPort(t, err, networkConf.ParallelServerProxyPort)
}

func verifyErrorContainsPort(t *testing.T, err error, port int) {
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", port), "Message did not mention expected port")
}

func addPortToService(svc *corev1.Service, port int) {
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Port:       int32(port),
		TargetPort: intstr.FromInt(port),
	})
}
