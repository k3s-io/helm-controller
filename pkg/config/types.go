package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type CLI struct {
	Debug           bool
	DebugLevel      int
	Kubeconfig      string
	MasterURL       string
	Namespace       string
	Threads         int
	ControllerName  string
	NodeName        string
	JobClusterRole  string
	DefaultJobImage string
	JobTolerations  string
	JobResources    string
	PprofPort       int
}

func (c CLI) GetControllerConfig() (*Controller, error) {
	resources, err := parseResources(c.JobResources)
	if err != nil {
		return nil, fmt.Errorf("invalid job-resources JSON: %w", err)
	}
	tolerations, err := parseTolerations(c.JobTolerations)
	if err != nil {
		return nil, fmt.Errorf("invalid --job-tolerations JSON: %w", err)
	}

	if c.Threads <= 0 {
		return nil, fmt.Errorf("cannot start with thread count of %d, please pass a proper thread count", c.Threads)
	}

	return &Controller{
		Threadiness:     c.Threads,
		NodeName:        c.NodeName,
		JobClusterRole:  c.JobClusterRole,
		DefaultJobImage: c.DefaultJobImage,
		JobTolerations:  tolerations,
		JobResources:    resources,
	}, nil
}

type Controller struct {
	Threadiness     int
	NodeName        string
	JobClusterRole  string
	DefaultJobImage string
	JobTolerations  []corev1.Toleration
	JobResources    *corev1.ResourceRequirements
}
