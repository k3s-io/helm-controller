package app

import (
	"github.com/k3s-io/helm-controller/pkg/cmd"
	"github.com/k3s-io/helm-controller/pkg/config"
	"github.com/k3s-io/helm-controller/pkg/version"
	"github.com/urfave/cli/v2"
)

var cliconfig config.CLI

func New() *cli.App {
	return &cli.App{
		Name:        "helm-controller",
		Description: "A simple way to manage helm charts with CRDs in K8s.",
		Version:     version.FriendlyVersion(),
		Action: func(app *cli.Context) error {
			return cmd.Run(app.Context, cliconfig)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "controller-name",
				Value:       "helm-controller",
				Usage:       "Unique name to identify this controller that is added to all HelmCharts tracked by this controller",
				EnvVars:     []string{"CONTROLLER_NAME"},
				Destination: &cliconfig.ControllerName,
			},
			&cli.BoolFlag{
				Name:        "debug",
				Usage:       "Turn on debug logging",
				Destination: &cliconfig.Debug,
			},
			&cli.IntFlag{
				Name:        "debug-level",
				Usage:       "If debugging is enabled, set klog -v=X",
				Destination: &cliconfig.DebugLevel,
			},
			&cli.BoolFlag{
				Name:        "enforce-pod-limits",
				Value:       true,
				Usage:       "Set to false to disable default CPU and memory limits on the pods created to manage helm charts",
				EnvVars:     []string{"ENFORCE_POD_LIMITS"},
				Destination: &cliconfig.EnforcePodLimits,
			},
			&cli.StringFlag{
				Name:        "default-job-image",
				Usage:       "Default image to use by jobs managing helm charts",
				EnvVars:     []string{"DEFAULT_JOB_IMAGE"},
				Destination: &cliconfig.DefaultJobImage,
			},
			&cli.StringFlag{
				Name:        "job-tolerations",
				Usage:       "JSON array of tolerations to apply to all jobs managing helm charts",
				EnvVars:     []string{"JOB_TOLERATIONS"},
				Destination: &cliconfig.JobTolerations,
			},
			&cli.StringFlag{
				Name:        "job-cluster-role",
				Value:       "cluster-admin",
				Usage:       "Name of the cluster role to use for jobs created to manage helm charts",
				EnvVars:     []string{"JOB_CLUSTER_ROLE"},
				Destination: &cliconfig.JobClusterRole,
			},
			&cli.StringFlag{
				Name:        "kubeconfig",
				Usage:       "Kubernetes config files, e.g. $HOME/.kube/config",
				EnvVars:     []string{"KUBECONFIG"},
				Destination: &cliconfig.Kubeconfig,
			},
			&cli.StringFlag{
				Name:        "master-url",
				Usage:       "Kubernetes cluster master URL",
				EnvVars:     []string{"MASTERURL"},
				Destination: &cliconfig.MasterURL,
			},
			&cli.StringFlag{
				Name:        "namespace",
				Usage:       "Namespace to watch, empty means it will watch CRDs in all namespaces",
				EnvVars:     []string{"NAMESPACE"},
				Destination: &cliconfig.Namespace,
			},
			&cli.StringFlag{
				Name:        "node-name",
				Usage:       "Name of the node this controller is running on",
				EnvVars:     []string{"NODE_NAME"},
				Destination: &cliconfig.NodeName,
			},
			&cli.IntFlag{
				Name:        "pprof-port",
				Value:       6060,
				Usage:       "Port to publish HTTP server runtime profiling data in the format expected by the pprof visualization tool. Only enabled if in debug mode",
				Destination: &cliconfig.PprofPort,
			},
			&cli.IntFlag{
				Name:        "threads",
				Value:       2,
				Usage:       "Threadiness level to set",
				EnvVars:     []string{"THREADS"},
				Destination: &cliconfig.Threads,
			},
		},
	}
}

// Config returns the controller config provided by parsing the provided CLI flags.
func Config(args []string) (*config.Controller, error) {
	a := New()
	a.Action = func(*cli.Context) error { return nil }
	a.Run(append([]string{a.Name}, args...))
	return cliconfig.GetControllerConfig()
}
