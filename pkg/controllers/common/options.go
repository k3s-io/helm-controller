package common

import "fmt"

// Options defines options that can be set on initializing the Helm Controller
type Options struct {
	Threadiness int
	NodeName    string
}

func (opts Options) Validate() error {
	if opts.Threadiness <= 0 {
		return fmt.Errorf("cannot start with thread count of %d, please pass a proper thread count", opts.Threadiness)
	}
	return nil
}
