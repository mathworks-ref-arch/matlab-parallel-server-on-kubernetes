// Copyright 2025 The MathWorks, Inc.
package jsonmarshal_test

import (
	"controller/internal/jsonmarshal"
	"testing"

	"github.com/mathworks/mjssetup/pkg/profile"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonMarshal(t *testing.T) {
	input := &profile.Profile{
		Name: "my-profile",
		SchedulerComponent: profile.SchedComp{
			Host:        "clusterhost",
			Certificate: "mycert",
		},
	}
	marshaller := jsonmarshal.New()
	marshalled, err := marshaller.Marshal(input)
	require.NoError(t, err, "Failed to marshal")
	gotProf, err := marshaller.UnmarshalProfile(marshalled)
	require.NoError(t, err, "Failed to unmarshal")
	assert.Equal(t, input, gotProf, "Marshalled and unmarshalled objects should be equal")
}
