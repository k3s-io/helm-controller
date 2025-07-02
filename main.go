//go:generate go run pkg/codegen/cleanup/main.go
//go:generate rm -rf pkg/generated pkg/crds/yaml/generated
//go:generate go run pkg/codegen/main.go
//go:generate controller-gen crd:generateEmbeddedObjectMeta=true paths=./pkg/apis/... output:crd:dir=./pkg/crds/yaml/generated
//go:generate crd-ref-docs --config=crd-ref-docs.yaml --renderer=markdown --output-path=doc/helmchart.md

package main

import (
	_ "net/http/pprof"
	"os"

	"github.com/k3s-io/helm-controller/pkg/cmd"
	"github.com/k3s-io/helm-controller/pkg/version"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var config = cmd.HelmController{}

func main() {
	app := &cli.App{
		Name:        "helm-controller",
		Description: "A simple way to manage helm charts with CRDs in K8s.",
		Version:     version.FriendlyVersion(),
		Action: func(app *cli.Context) error {
			return config.Run(app)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "controller-name",
				Value:       "helm-controller",
				Usage:       "Unique name to identify this controller that is added to all HelmCharts tracked by this controller",
				EnvVars:     []string{"CONTROLLER_NAME"},
				Destination: &config.ControllerName,
			},
			&cli.BoolFlag{
				Name:        "debug",
				Usage:       "Turn on debug logging",
				Destination: &config.Debug,
			},
			&cli.IntFlag{
				Name:        "debug-level",
				Usage:       "If debugging is enabled, set klog -v=X",
				Destination: &config.DebugLevel,
			},
			&cli.StringFlag{
				Name:        "default-job-image",
				Usage:       "Default image to use by jobs managing helm charts",
				EnvVars:     []string{"DEFAULT_JOB_IMAGE"},
				Destination: &config.DefaultJobImage,
			},
			&cli.StringFlag{
				Name:        "job-cluster-role",
				Value:       "cluster-admin",
				Usage:       "Name of the cluster role to use for jobs created to manage helm charts",
				EnvVars:     []string{"JOB_CLUSTER_ROLE"},
				Destination: &config.JobClusterRole,
			},
			&cli.StringFlag{
				Name:        "kubeconfig",
				Usage:       "Kubernetes config files, e.g. $HOME/.kube/config",
				EnvVars:     []string{"KUBECONFIG"},
				Destination: &config.Kubeconfig,
			},
			&cli.StringFlag{
				Name:        "master-url",
				Usage:       "Kubernetes cluster master URL",
				EnvVars:     []string{"MASTERURL"},
				Destination: &config.MasterURL,
			},
			&cli.StringFlag{
				Name:        "namespace",
				Usage:       "Namespace to watch, empty means it will watch CRDs in all namespaces",
				EnvVars:     []string{"NAMESPACE"},
				Destination: &config.Namespace,
			},
			&cli.StringFlag{
				Name:        "node-name",
				Usage:       "Name of the node this controller is running on",
				EnvVars:     []string{"NODE_NAME"},
				Destination: &config.NodeName,
			},
			&cli.IntFlag{
				Name:        "pprof-port",
				Value:       6060,
				Usage:       "Port to publish HTTP server runtime profiling data in the format expected by the pprof visualization tool. Only enabled if in debug mode",
				Destination: &config.PprofPort,
			},
			&cli.IntFlag{
				Name:        "threads",
				Value:       2,
				Usage:       "Threadiness level to set",
				EnvVars:     []string{"THREADS"},
				Destination: &config.Threads,
			},
		},
	}

	ctx := signals.SetupSignalContext()
	if err := app.RunContext(ctx, os.Args); err != nil {
		logrus.Fatal(err)
	}
}
