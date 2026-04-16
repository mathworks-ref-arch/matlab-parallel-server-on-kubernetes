// Copyright 2024-2025 The MathWorks, Inc.
package request_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"controller/internal/controller"
	"controller/internal/logging"
	"controller/internal/request"
	mockClient "controller/mocks/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test construction of the resize status command
func TestGetResizeStatusCommand(t *testing.T) {
	conf := request.ExecConfig{
		BasePort:                  27350,
		RequireScriptVerification: false,
		ResizePath:                "/path/to/resize",
		TimeoutSecs:               10,
	}
	cmd := request.GetResizeStatusCommand(conf)
	require.Len(t, cmd, 6, "Unexpected command length")
	verifyResizeStatusCommand(t, conf, cmd)
}

// Test construction of the resize status command with script verification turned on
func TestGetResizeStatusCommandWithVerification(t *testing.T) {
	conf := request.ExecConfig{
		BasePort:                  3000,
		RequireScriptVerification: true,
		ResizePath:                "/path/to/resize",
		SecretPath:                "/path/to/secret",
		TimeoutSecs:               2,
	}
	cmd := request.GetResizeStatusCommand(conf)
	require.Len(t, cmd, 8, "Unexpected command length")
	verifyResizeStatusCommand(t, conf, cmd)
	assert.Equal(t, "-secretfile", cmd[6], "Command should contain secret file arg")
	assert.Equal(t, conf.SecretPath, cmd[7], "Command should contain path to secret file arg")
}

func TestGetRequestNoWorkers(t *testing.T) {
	requestGetter, client := newWithMock(t, nil)

	// Construct expected response
	wantReq := controller.ResizeRequest{
		DesiredWorkers: 5,
		MaxWorkers:     20,
		Workers:        []controller.ConnectedWorker{},
	}

	// Create the raw string needed to get the expected request
	rawReq := fmt.Sprintf(`
{
	"jobManagers": [
		{
		"name": "myJobManager",
		"host": "myhostname",
		"desiredWorkers": {
			"linux": %d,
			"windows": 0
		},
		"maxWorkers": {
			"linux": %d,
			"windows": 8
		},
		"workers": []
		}
	]
}`, wantReq.DesiredWorkers, wantReq.MaxWorkers)

	// Check we get the expected request
	expectExecReponse(client, rawReq)
	gotReq, err := requestGetter.GetRequest()
	require.NoError(t, err)
	assert.Equal(t, wantReq, *gotReq, "unexpectected resize request returned")
}

func TestGetRequestWithWorkers(t *testing.T) {
	requestGetter, client := newWithMock(t, nil)

	// Construct expected response
	wantReq := controller.ResizeRequest{
		DesiredWorkers: 10,
		MaxWorkers:     50,
		Workers: []controller.ConnectedWorker{
			{
				Name:        "worker1",
				State:       "busy",
				SecondsIdle: 0,
			},
			{
				Name:        "worker2",
				State:       "idle",
				SecondsIdle: 30,
			},
		},
	}

	// Create the raw string needed to get the expected request
	rawReq := fmt.Sprintf(`
{
	"jobManagers": [
		{
		"name": "myJobManager",
		"host": "myhostname",
		"desiredWorkers": {
			"linux": %d,
			"windows": 0
		},
		"maxWorkers": {
			"linux": %d,
			"windows": 8
		},
		"workers": [
			{
			"name": "%s",
			"host": "myhostname",
			"operatingSystem": "linux",
			"state": "%s",
			"secondsIdle": %d
			},
			{
			"name": "%s",
			"host": "myhostname",
			"operatingSystem": "linux",
			"state": "%s",
			"secondsIdle": %d
			}
		]
		}
	]
}`, wantReq.DesiredWorkers, wantReq.MaxWorkers, wantReq.Workers[0].Name, wantReq.Workers[0].State, wantReq.Workers[0].SecondsIdle, wantReq.Workers[1].Name, wantReq.Workers[1].State, wantReq.Workers[1].SecondsIdle)

	// Check we get the expected request
	expectExecReponse(client, rawReq)
	gotReq, err := requestGetter.GetRequest()
	require.NoError(t, err)
	assert.Equal(t, wantReq, *gotReq, "unexpectected resize request returned")
}

func TestGetJobManagerPodErr(t *testing.T) {
	requestGetter, client := newWithMock(t, nil)
	errMsg := "could not get job manager pod"
	client.EXPECT().GetReadyPod(testConfig.JobManagerLabel, testConfig.JobManagerContainer).Once().Return(nil, errors.New(errMsg))
	_, err := requestGetter.GetRequest()
	assert.Error(t, err, "should get error when unable to get job manager pod")
	assert.Contains(t, err.Error(), errMsg, "error should contain original error message")
}

func TestExecError(t *testing.T) {
	requestGetter, client := newWithMock(t, nil)
	podName := "jm-pod"
	client.EXPECT().GetReadyPod(testConfig.JobManagerLabel, testConfig.JobManagerContainer).Once().Return(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}}, nil)
	errMsg := "could not exec the command"
	client.EXPECT().ExecOnPod(podName, testConfig.JobManagerContainer, testCmd).Once().Return(nil, errors.New(errMsg))
	_, err := requestGetter.GetRequest()
	assert.Error(t, err, "should get error when ExecOnPod errors")
	assert.Contains(t, err.Error(), errMsg, "error should contain original error message")
}

// Check for errors when processing a request that is not valid JSON
func TestInvalidJSON(t *testing.T) {
	invalidJSON := "this-is-not-valid"
	r, client := newWithMock(t, nil)
	expectExecReponse(client, invalidJSON)
	_, err := r.GetRequest()
	assert.Error(t, err, "Expect error when JSON cannot be unmarshaled")
}

// Check that we get an error when there is no job manager
func TestNoJobManagers(t *testing.T) {
	rawReq := `
{
	"jobManagers": [
	]
}`
	r, client := newWithMock(t, nil)
	expectExecReponse(client, rawReq)
	_, err := r.GetRequest()
	assert.Error(t, err, "Expect error when there are no job managers in response")
}

// Check that we exit when the request contains multiple job managers, which is not allowed
func TestProcessJSONMultipleJobManagers(t *testing.T) {
	rawReq := `
{
	"jobManagers": [
		{
			"name": "manager1"
		},
		{
			"name": "manager2"
		}
	]
}`
	didExit := false
	exitFunc := func(error) { didExit = true }
	m, client := newWithMock(t, exitFunc)
	expectExecReponse(client, rawReq)
	_, err := m.GetRequest()
	require.NoError(t, err)
	assert.True(t, didExit, "Process should have exited when multiple job managers were found")
}

func newWithMock(t *testing.T, exitFunc func(error)) (controller.ResizeRequestGetter, *mockClient.MockClient) {
	client := mockClient.NewMockClient(t)
	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))
	return request.NewPodResizeRequestGetter(testConfig, client, exitFunc, logger), client
}

func verifyResizeStatusCommand(t *testing.T, conf request.ExecConfig, cmd []string) {
	assert.Equal(t, "timeout", cmd[0], "Command should start with timeout")
	assert.Equal(t, fmt.Sprintf("%d", conf.TimeoutSecs), cmd[1], "Command should include timeout in seconds")
	assert.Equal(t, conf.ResizePath, cmd[2], "Command should include path to resize script")
	assert.Equal(t, "status", cmd[3], "Command should include 'status' subcommand")
	assert.Equal(t, "-baseport", cmd[4], "Command should include baseport arg")
	assert.Equal(t, fmt.Sprintf("%d", conf.BasePort), cmd[5], "Command should include baseport value")
}

var testConfig = request.ExecConfig{
	TimeoutSecs:         30,
	ResizePath:          "/path/to/resize/script",
	BasePort:            8080,
	JobManagerContainer: "my-jm-container",
	JobManagerLabel:     "app=job-manager",
}

var testCmd = request.GetResizeStatusCommand(testConfig)

func expectExecReponse(client *mockClient.MockClient, response string) {
	// Configure client to get the job manager pod
	podName := "jm-pod"
	client.EXPECT().GetReadyPod(testConfig.JobManagerLabel, testConfig.JobManagerContainer).Once().Return(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}}, nil)

	// Configure client to make the request
	client.EXPECT().ExecOnPod(podName, testConfig.JobManagerContainer, testCmd).Return(bytes.NewBufferString(response), nil).Once()
}
