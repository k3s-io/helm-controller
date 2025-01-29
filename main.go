package main

import (
	_ "net/http/pprof"

	"github.com/k3s-io/helm-controller/pkg/cli"
	"github.com/k3s-io/helm-controller/pkg/version"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var config = cli.HelmController{}

func main() {
	cmd := &cobra.Command{
		Version: version.FriendlyVersion(),
	}

	cmd.Flags().StringVar(&config.ControllerName, "controller-name", "helm-controller", "Unique name to identify this controller that is added to all HelmCharts tracked by this controller. May be set via CONTROLLER_NAME env var.")
	cli.SetFlagtoEnv(cmd, "controller-name", "CONTROLLER_NAME")
	cmd.Flags().BoolVar(&config.Debug, "debug", false, "Turn on debug logging")
	cmd.Flags().IntVar(&config.DebugLevel, "debug-level", 0, "If debugging is enabled, set klog -v=X")
	cmd.Flags().StringVar(&config.DefaultJobImage, "default-job-image", "", "Default image to use by jobs managing helm charts. May be set via DEFAULT_JOB_IMAGE env var.")
	cli.SetFlagtoEnv(cmd, "default-job-image", "DEFAULT_JOB_IMAGE")
	cmd.Flags().StringVar(&config.JobClusterRole, "job-cluster-role", "cluster-admin", "Name of the cluster role to use for jobs created to manage helm charts. May be set via JOB_CLUSTER_ROLE env var.")
	cli.SetFlagtoEnv(cmd, "job-cluster-role", "JOB_CLUSTER_ROLE")
	cmd.Flags().StringVarP(&config.Kubeconfig, "kubeconfig", "k", "", "Kubernetes config files, e.g. $HOME/.kube/config. May be set via KUBECONFIG env var.")
	cli.SetFlagtoEnv(cmd, "kubeconfig", "KUBECONFIG")
	cmd.Flags().StringVarP(&config.MasterURL, "master-url", "m", "", "Kubernetes cluster master URL. May be set via MASTERURL env var.")
	cli.SetFlagtoEnv(cmd, "master-url", "MASTERURL")
	cmd.Flags().StringVarP(&config.Namespace, "namespace", "n", "", "Namespace to watch, empty means it will watch CRDs in all namespaces. May be set via NAMESPACE env var.")
	cli.SetFlagtoEnv(cmd, "namespace", "NAMESPACE")
	cmd.Flags().StringVar(&config.NodeName, "node-name", "", "Name of the node this controller is running on. May be set via NODE_NAME env var.")
	cli.SetFlagtoEnv(cmd, "node-name", "NODE_NAME")
	cmd.Flags().IntVar(&config.PprofPort, "pprof-port", 6060, "Port to publish HTTP server runtime profiling data in the format expected by the pprof visualization tool. Only enabled if in debug mode.")
	cmd.Flags().IntVarP(&config.Threads, "threads", "t", 2, "Threadiness level to set. May be set via THREADS env var.")
	cli.SetFlagtoEnv(cmd, "threads", "THREADS")
	cmd.RunE = config.Run

	ctx := signals.SetupSignalContext()
	if err := cmd.ExecuteContext(ctx); err != nil {
		logrus.Fatal(err)
	}
}
