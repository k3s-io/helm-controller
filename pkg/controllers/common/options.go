package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// Options defines options that can be set on initializing the Helm Controller
type Options struct {
	Threadiness      int
	NodeName         string
	JobClusterRole   string
	DefaultJobImage  string
	JobTolerations   []corev1.Toleration
	EnforcePodLimits bool
}

func (opts Options) Validate() error {
	if opts.Threadiness <= 0 {
		return fmt.Errorf("cannot start with thread count of %d, please pass a proper thread count", opts.Threadiness)
	}
	return nil
}
