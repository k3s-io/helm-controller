package config

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	typedcore "k8s.io/kubernetes/pkg/apis/core"
	typedcorev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

var scheme = runtime.NewScheme()
var opts = validation.PodValidationOptions{}

func init() {
	typedcorev1.RegisterConversions(scheme)
}

func parseResources(raw string) (*corev1.ResourceRequirements, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	resources := &corev1.ResourceRequirements{}
	if err := json.Unmarshal([]byte(raw), resources); err != nil {
		return nil, err
	}
	coreResources := &typedcore.ResourceRequirements{}
	if err := scheme.Convert(resources, coreResources, nil); err != nil {
		return nil, err
	}
	if err := validation.ValidateContainerResourceRequirements(coreResources, nil, field.NewPath("pod-resources"), opts).ToAggregate(); err != nil {
		return nil, err
	}
	return resources, nil
}

// parseTolerations takes the CLI string and parses it into a slice of corev1.Toleration objects
func parseTolerations(raw string) ([]corev1.Toleration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	tolerations := []corev1.Toleration{}
	if err := json.Unmarshal([]byte(raw), &tolerations); err != nil {
		return nil, err
	}
	coreTolerations := []typedcore.Toleration{}
	for _, t := range tolerations {
		c := typedcore.Toleration{}
		if err := scheme.Convert(&t, &c, nil); err != nil {
			return nil, err
		}
		coreTolerations = append(coreTolerations, c)
	}
	if err := validation.ValidateTolerations(coreTolerations, field.NewPath("job-tolerations")).ToAggregate(); err != nil {
		return nil, err
	}
	return tolerations, nil
}
