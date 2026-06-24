package config

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// parseTolerations takes the CLI string and parses it into a slice of corev1.Toleration objects
func parseTolerations(raw string) ([]corev1.Toleration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var tolerations []corev1.Toleration
	if err := json.Unmarshal([]byte(raw), &tolerations); err != nil {
		return nil, err
	}
	return tolerations, nil
}
