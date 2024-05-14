// Copyright 2024 The MathWorks, Inc.
package request

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"controller/internal/config"
	"controller/internal/logging"
	mockClient "controller/mocks/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test the GetRequest method
func TestGetRequest(t *testing.T) {
	verifyGetRequest(t, false)
}

// Test the GetRequest method when requireScriptVerification=trye
func TestGetRequestWithScriptVerification(t *testing.T) {
	verifyGetRequest(t, true)
}

func verifyGetRequest(t *testing.T, requireScriptVerification bool) {
	conf := config.Config{
		ResizePath:                "/path/to/resize/script",
		RequireScriptVerification: requireScriptVerification,
		SecretDir:                 "/my/secret",
		SecretFileName:            "secret.json",
	}
	requestGetter, client := createRequestGetterWithMockClient(t, &conf)

	// Construct expected response
	wantReq := ResizeRequest{
		DesiredWorkers: 10,
		MaxWorkers:     50,
		Workers: []WorkerStatus{
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

	// Set up mock client to return the job manager pod
	jmPodName := "jmPod"
	client.EXPECT().GetJobManagerPod().Once().Return(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: jmPodName},
	}, nil)

	// Check the command we are going to run
	expectedCmd := getResizeStatusCommand(requestGetter.config)
	secretFileArg := "-secretfile"
	assert.Contains(t, expectedCmd, conf.ResizePath, "command should contain path to resize executable")
	if requireScriptVerification {
		assert.Containsf(t, expectedCmd, secretFileArg, "resize status command should contain %s when requireScriptVerification is true", secretFileArg)
	} else {
		assert.NotContainsf(t, expectedCmd, secretFileArg, "resize status command should not contain %s when requireScriptVerification is false", secretFileArg)
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

	// Set up the mock client to return this string
	stdOut := bytes.NewBuffer([]byte(rawReq))
	client.EXPECT().ExecOnPod(jmPodName, expectedCmd).Once().Return(stdOut, nil)

	// Check we get the expected request
	gotReq, err := requestGetter.GetRequest()
	require.NoError(t, err)
	assert.Equal(t, wantReq, *gotReq, "unexpectected resize request returned")
}

func TestGetJobManagerPodErr(t *testing.T) {
	requestGetter, client := createRequestGetterWithMockClient(t, &config.Config{})
	errMsg := "could not get job manager pod"
	client.EXPECT().GetJobManagerPod().Once().Return(nil, errors.New(errMsg))
	_, err := requestGetter.GetRequest()
	assert.Error(t, err, "should get error when GetJobManagerPod errors")
	assert.Contains(t, err.Error(), errMsg, "error should contain original error message")
}

func TestExecError(t *testing.T) {
	requestGetter, client := createRequestGetterWithMockClient(t, &config.Config{})
	podName := "jm-pod"
	client.EXPECT().GetJobManagerPod().Once().Return(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}}, nil)
	errMsg := "could not exec the command"
	client.EXPECT().ExecOnPod(podName, mock.Anything).Once().Return(nil, errors.New(errMsg))
	_, err := requestGetter.GetRequest()
	assert.Error(t, err, "should get error when GetJobManagerPod errors")
	assert.Contains(t, err.Error(), errMsg, "error should contain original error message")
}

// Check for errors when processing a request that is not valid JSON
func TestProcessStatusInvalidJSON(t *testing.T) {
	invalidJSON := "this-is-not-valid"
	m, _ := createRequestGetterWithMockClient(t, &config.Config{})
	status, err := m.processRequest([]byte(invalidJSON))
	assert.Error(t, err, "processStatus should error when JSON cannot be unmarshaled")
	assert.Nil(t, status, "processStatus should return nil status when JSON cannot be unmarshaled")
}

// Check that we get an error when there is no job manager
func TestNoJobManagers(t *testing.T) {
	rawReq := `
{
	"jobManagers": [
	]
}`
	m, _ := createRequestGetterWithMockClient(t, &config.Config{})
	status, err := m.processRequest([]byte(rawReq))
	assert.Error(t, err, "processStatus should error when JSON cannot be unmarshaled")
	assert.Nil(t, status, "processStatus should return nil status when JSON cannot be unmarshaled")
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
	m, _ := createRequestGetterWithMockClient(t, &config.Config{})
	didExit := false
	m.exitFunc = func() { didExit = true }
	m.processRequest([]byte(rawReq))
	assert.True(t, didExit, "Process should have exited when multiple job managers were found")
}

func createRequestGetterWithMockClient(t *testing.T, conf *config.Config) (*MJSRequestGetter, *mockClient.Client) {
	client := mockClient.NewClient(t)
	return &MJSRequestGetter{
		client: client,
		logger: logging.NewFromZapLogger(zaptest.NewLogger(t)),
		config: conf,
	}, client
}
