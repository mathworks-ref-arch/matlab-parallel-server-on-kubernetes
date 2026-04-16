// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"fmt"
	"math"

	"go.uber.org/zap"
)

// Check that the load balancer service exists and exposes the correct ports
func (s *Setup) CheckLoadBalancer() error {
	if s.network.InternalClientsOnly {
		// There is no load balancer for internal-only mode
		return nil
	}

	lbName := s.resources.LoadBalancer
	s.logger.Info("checking load balancer", zap.String("name", lbName))
	svc, exists, err := s.client.ServiceExists(lbName)
	if err != nil {
		return err
	}

	// Compute the ports we expect the load balancer to expose
	var requiredPorts []int
	if s.network.UsePoolProxy {
		requiredPorts = []int{s.network.LookupPort, s.network.JobManagerPort}
		maxPoolProxies := int(math.Ceil(float64(s.network.MaxWorkers) / float64(s.network.WorkersPerPoolProxy)))
		for i := 0; i < maxPoolProxies; i++ {
			requiredPorts = append(requiredPorts, s.network.PoolProxyBasePort+i)
		}
	}
	if s.network.UseParallelServerProxy {
		requiredPorts = []int{s.network.ParallelServerProxyPort}
	}

	// Error if the load balancer does not exist
	if !exists {
		portPairs := ""
		for idx, p := range requiredPorts {
			if idx > 0 {
				portPairs += ","
			}
			portPairs += fmt.Sprintf("%d:%d", p, p)
		}
		exampleCmd := fmt.Sprintf("kubectl create service loadbalancer %s --namespace %s --tcp %s", lbName, s.network.Namespace, portPairs)
		return fmt.Errorf(`error: Load balancer service "%s" does not exist in namespace "%s". Create a load balancer service configured for MATLAB Job Scheduler with command: "%s"`, lbName, s.network.Namespace, exampleCmd)
	}

	// If the service exists, check that all ports are exposed correctly
	exposedPorts := map[int]bool{}
	for _, p := range svc.Spec.Ports {
		port := int(p.Port)
		targetPort := p.TargetPort.IntValue()
		if port != targetPort {
			return fmt.Errorf(`error: Target port %d does not match service port %d in specification for load balancer service "%s". Modify the service specification so that all target ports match service ports`, targetPort, port, lbName)
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
		return fmt.Errorf(`error: Load balancer service "%s" does not expose all ports required by MATLAB Job Scheduler. Missing ports: %s. Modify the service specification to expose all required ports`, lbName, missingPorts)
	}

	s.logger.Info("load balancer is correctly configured")
	return nil
}
