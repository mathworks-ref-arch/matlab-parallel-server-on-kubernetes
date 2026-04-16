// Implementation of marshalling an object into JSON bytes.
// Copyright 2025-2026 The MathWorks, Inc.
package jsonmarshal

import (
	"controller/internal/setup"
	"encoding/json"

	"github.com/mathworks/mjssetup/pkg/profile"
)

type JsonMarshaller struct{}

func New() setup.Marshaller {
	return &JsonMarshaller{}
}

func (j *JsonMarshaller) Marshal(input any) ([]byte, error) {
	return json.MarshalIndent(input, "", "  ")
}

func (j *JsonMarshaller) UnmarshalProfile(data []byte) (*profile.Profile, error) {
	prof := &profile.Profile{}
	err := json.Unmarshal(data, prof)
	if err != nil {
		return nil, err
	}
	return prof, nil
}
