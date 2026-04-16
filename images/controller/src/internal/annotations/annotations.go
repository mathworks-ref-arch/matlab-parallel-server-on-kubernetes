// Helper functions for extracting values from annotations on Kubernetes resources.
// Copyright 2026 The MathWorks, Inc.
package annotations

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SetStringsFromAnnotations(toSet map[string]*string, meta *metav1.ObjectMeta) error {
	for label, toSet := range toSet {
		val, err := GetAnnotationString(label, meta)
		if err != nil {
			return err
		}
		*toSet = val
	}
	return nil
}

func SetIntsFromAnnotations(toSet map[string]*int, meta *metav1.ObjectMeta) error {
	for label, toSet := range toSet {
		val, err := GetAnnotationInt(label, meta)
		if err != nil {
			return err
		}
		*toSet = val
	}
	return nil
}

func SetBoolsFromAnnotations(toSet map[string]*bool, meta *metav1.ObjectMeta) error {
	for label, toSet := range toSet {
		val, err := GetAnnotationBool(label, meta)
		if err != nil {
			return err
		}
		*toSet = val
	}
	return nil
}

func GetAnnotationString(label string, meta *metav1.ObjectMeta) (string, error) {
	val, ok := meta.Annotations[label]
	if !ok {
		return "", fmt.Errorf("required annotation %s missing from pod template", label)
	}
	return val, nil
}

func GetOptionalAnnotationString(label string, meta *metav1.ObjectMeta) (string, bool) {
	val, ok := meta.Annotations[label]
	return val, ok
}

func GetAnnotationInt(label string, meta *metav1.ObjectMeta) (int, error) {
	val, ok, err := GetOptionalAnnotationInt(label, meta)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("required annotation %s missing from pod template", label)
	}
	return val, nil
}

func GetOptionalAnnotationInt(label string, meta *metav1.ObjectMeta) (int, bool, error) {
	valStr, ok := GetOptionalAnnotationString(label, meta)
	if !ok {
		return 0, ok, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, false, fmt.Errorf("failed to parse annotation '%s=%s' as integer: %v", label, valStr, err)
	}
	return val, true, nil
}

func GetAnnotationBool(label string, meta *metav1.ObjectMeta) (bool, error) {
	valStr, err := GetAnnotationString(label, meta)
	if err != nil {
		return false, err
	}
	tf, err := strconv.ParseBool(valStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse annotation '%s=%s' as boolean: %v", label, valStr, err)
	}
	return tf, nil
}
