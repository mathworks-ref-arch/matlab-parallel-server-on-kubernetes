// Copyright 2024 The MathWorks, Inc.
package rescaler

import (
	"controller/internal/logging"
	"controller/internal/request"
	"controller/internal/resize"
	"controller/internal/specs"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	mockRequest "controller/mocks/request"
	mockResize "controller/mocks/resize"
)

// Test the Rescale method against mock backends
func TestRescale(t *testing.T) {
	idleStopThreshold := 10
	testCases := []struct {
		desc            string
		existingWorkers []int
		idleTimes       []int
		desiredWorkers  int
		maxWorkers      int
		shouldDelete    []string
		shouldAdd       []int
	}{
		{
			"increase_workers",
			[]int{1},
			[]int{idleStopThreshold},
			2,
			10,
			[]string{},
			[]int{2},
		}, {
			"decrease_workers",
			[]int{1, 2},
			[]int{idleStopThreshold, idleStopThreshold},
			1,
			10,
			[]string{"mjs-worker-2"},
			[]int{},
		}, {
			"workers_already_max",
			[]int{1, 2},
			[]int{idleStopThreshold, idleStopThreshold},
			10,
			2,
			[]string{},
			[]int{},
		}, {
			"increase_up_to_max",
			[]int{1},
			[]int{idleStopThreshold},
			10,
			2,
			[]string{},
			[]int{2},
		}, {
			"decrease_idle_too_short",
			[]int{1},
			[]int{idleStopThreshold - 1},
			0,
			10,
			[]string{},
			[]int{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			// Set up mock resize status
			states := []string{}
			for i := 0; i < len(testCase.existingWorkers); i++ {
				states = append(states, "idle")
			}
			infos := createWorkerInfos(testCase.existingWorkers)
			workers := createWorkerStatuses(t, infos, states, testCase.idleTimes)
			resizeOutput := request.ResizeRequest{
				DesiredWorkers: testCase.desiredWorkers,
				MaxWorkers:     testCase.maxWorkers,
				Workers:        workers,
			}
			mockRequestGetter := mockRequest.NewGetter(t)
			mockRequestGetter.EXPECT().GetRequest().Return(&resizeOutput, nil).Once()

			// Set up mock resizer
			mockResizer := mockResize.NewResizer(t)
			mockResizer.EXPECT().GetWorkers().Return(infos, nil).Once()
			if len(testCase.shouldDelete) > 0 {
				mockResizer.EXPECT().DeleteWorkers(testCase.shouldDelete).Return(nil).Once()
			}
			if len(testCase.shouldAdd) > 0 {
				toAdd := []specs.WorkerInfo{}
				for _, id := range testCase.shouldAdd {
					toAdd = append(toAdd, specs.WorkerInfo{
						Name: fmt.Sprintf("mjs-worker-%d", id),
						ID:   id,
					})
				}
				mockResizer.EXPECT().AddWorkers(toAdd).Return(nil).Once()
			}

			// Create scaler with mocks
			scaler := MJSRescaler{
				requestGetter:     mockRequestGetter,
				resizer:           mockResizer,
				idleStopThreshold: idleStopThreshold,
				logger:            logging.NewFromZapLogger(zaptest.NewLogger(t)),
			}
			scaler.Rescale()
		})
	}
}

func TestRescaleError(t *testing.T) {
	statusWithError := mockRequest.NewGetter(t)
	statusWithError.EXPECT().GetRequest().Return(nil, errors.New("error")).Once()
	mockResizer := mockResize.NewResizer(t)
	scaler := MJSRescaler{
		requestGetter:     statusWithError,
		resizer:           mockResizer,
		idleStopThreshold: 10,
		logger:            logging.NewFromZapLogger(zaptest.NewLogger(t)),
	}
	scaler.Rescale()
}

func TestRescaleResizerError(t *testing.T) {
	resizerWithError := mockResize.NewResizer(t)
	resizerWithError.EXPECT().GetWorkers().Return([]resize.Worker{}, fmt.Errorf("some error")).Once()

	mockRequestGetter := mockRequest.NewGetter(t)
	clusterStatus := request.ResizeRequest{
		DesiredWorkers: 2,
		MaxWorkers:     3,
		Workers: []request.WorkerStatus{
			{
				Name:        "mjs-worker-2",
				State:       "idle",
				SecondsIdle: 0,
			},
		},
	}
	mockRequestGetter.EXPECT().GetRequest().Return(&clusterStatus, nil).Once()

	scaler := MJSRescaler{
		requestGetter:     mockRequestGetter,
		resizer:           resizerWithError,
		idleStopThreshold: 10,
		logger:            logging.NewFromZapLogger(zaptest.NewLogger(t)),
	}
	scaler.Rescale()
}

// Test the logic of the getWorkersToDelete function in various scenarios
func TestGetWorkersToDelete(t *testing.T) {
	threshold := 30
	testCases := []struct {
		name         string
		numToDelete  int
		mjsWorkers   []request.WorkerStatus
		k8sWorkers   []resize.Worker
		shouldDelete []string
	}{
		{
			// Case where we cannot delete any workers because they're all busy
			name:        "all_workers_busy",
			numToDelete: 2,
			mjsWorkers: []request.WorkerStatus{
				{Name: "worker1", State: "busy", SecondsIdle: 0},
				{Name: "worker2", State: "busy", SecondsIdle: 0},
			},
			k8sWorkers: []resize.Worker{
				{Info: specs.WorkerInfo{Name: "worker1"}, IsRunning: true},
				{Info: specs.WorkerInfo{Name: "worker2"}, IsRunning: true},
			},
			shouldDelete: []string{},
		}, {
			// Case where we cannot delete any workers because none have been idle for long enough
			name:        "all_workers_below_idle_threshold",
			numToDelete: 1,
			mjsWorkers: []request.WorkerStatus{
				{Name: "worker1", State: "idle", SecondsIdle: threshold - 1},
				{Name: "worker2", State: "idle", SecondsIdle: threshold - 1},
			},
			k8sWorkers: []resize.Worker{
				{Info: specs.WorkerInfo{Name: "worker1"}, IsRunning: true},
				{Info: specs.WorkerInfo{Name: "worker2"}, IsRunning: true},
			},
			shouldDelete: []string{},
		}, {
			// Case where we delete some idle workers
			name:        "delete_some_idle_workers",
			numToDelete: 3,
			mjsWorkers: []request.WorkerStatus{
				{Name: "worker1", State: "idle", SecondsIdle: threshold + 1},
				{Name: "worker2", State: "idle", SecondsIdle: threshold - 1},
				{Name: "worker3", State: "idle", SecondsIdle: threshold + 1},
				{Name: "worker4", State: "busy", SecondsIdle: 0},
			},
			k8sWorkers: []resize.Worker{
				{Info: specs.WorkerInfo{Name: "worker1"}, IsRunning: true},
				{Info: specs.WorkerInfo{Name: "worker2"}, IsRunning: true},
				{Info: specs.WorkerInfo{Name: "worker3"}, IsRunning: true},
				{Info: specs.WorkerInfo{Name: "worker4"}, IsRunning: true},
			},
			shouldDelete: []string{"worker1", "worker3"},
		}, {
			// Case where we cannot delete workers that haven't connected yet (see g3278906)
			name:        "do_not_delete_unconnected_workers",
			numToDelete: 5,
			mjsWorkers: []request.WorkerStatus{
				{Name: "worker3", State: "busy", SecondsIdle: 0},
				{Name: "worker4", State: "idle", SecondsIdle: threshold + 1},
			},
			k8sWorkers: []resize.Worker{
				{Info: specs.WorkerInfo{Name: "worker1"}, IsRunning: false}, // Pod not running
				{Info: specs.WorkerInfo{Name: "worker2"}, IsRunning: true},  // Running but not connected to MJS yet
				{Info: specs.WorkerInfo{Name: "worker3"}, IsRunning: true},  // Running and connected to MJS
				{Info: specs.WorkerInfo{Name: "worker4"}, IsRunning: true},  // Running and connected to MJS
				{Info: specs.WorkerInfo{Name: "worker5"}, IsRunning: false}, // Pod not running
			},
			shouldDelete: []string{"worker4"},
		}, {
			// Case where some workers are still connected to MJS but have already been deleted from K8s, so we should not try to delete them again
			name:        "workers_already_removed_from_k8s",
			numToDelete: 2,
			mjsWorkers: []request.WorkerStatus{
				{Name: "worker3", State: "idle", SecondsIdle: threshold + 1},
				{Name: "worker4", State: "idle", SecondsIdle: threshold + 1},
			},
			k8sWorkers: []resize.Worker{
				{Info: specs.WorkerInfo{Name: "worker3"}, IsRunning: true},
				// No K8s entry for worker4 - it is already being terminated
			},
			shouldDelete: []string{"worker3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			toDelete := getWorkersToDelete(tc.numToDelete, tc.mjsWorkers, tc.k8sWorkers, threshold)
			assert.LessOrEqual(t, len(toDelete), tc.numToDelete, "Number of workers to delete should never be greater than the requested number to delete")
			require.ElementsMatch(t, tc.shouldDelete, toDelete, "Unexpected result from getWorkerToDelete")
		})
	}
}

func createWorkerStatuses(t *testing.T, inputWorkers []resize.Worker, states []string, idleTimes []int) []request.WorkerStatus {
	assert.Equal(t, len(inputWorkers), len(states), "states must be same length as input workers")
	assert.Equal(t, len(inputWorkers), len(idleTimes), "idleTimes must be same length as input workers")
	statuses := []request.WorkerStatus{}
	for idx, w := range inputWorkers {
		workerStatus := request.WorkerStatus{
			Name:        w.Info.Name,
			State:       states[idx],
			SecondsIdle: idleTimes[idx],
		}
		statuses = append(statuses, workerStatus)
	}
	return statuses
}

func createWorkerInfos(ids []int) []resize.Worker {
	workers := []resize.Worker{}
	for _, id := range ids {
		workers = append(workers, resize.Worker{
			Info: specs.WorkerInfo{
				Name: fmt.Sprintf("mjs-worker-%d", id),
				ID:   id,
			}})
	}
	return workers
}
