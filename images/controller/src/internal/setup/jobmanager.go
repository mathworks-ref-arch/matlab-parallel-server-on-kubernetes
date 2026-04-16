// Copyright 2025 The MathWorks, Inc.
package setup

import "time"

// Return when the job manager pod is ready
func (s *Setup) WaitForJobManager() error {
	s.logger.Info("waiting for job manager pod to be ready")
	period := time.Duration(s.config.JobManagerWaitPeriod) * time.Second
	for {
		isReady, err := s.client.IsPodReady(s.resources.JobManagerLabel, s.resources.JobManagerContainer)
		if err != nil {
			return err
		}
		if isReady {
			break
		}
		time.Sleep(period)
	}
	s.logger.Info("found ready job manager pod")
	return nil
}
