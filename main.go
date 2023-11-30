package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/k3s-io/helm-controller/pkg/controllers"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/crd"
	"github.com/k3s-io/helm-controller/pkg/version"
	command "github.com/rancher/wrangler-cli"
	wcrd "github.com/rancher/wrangler/pkg/crd"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	debugConfig command.DebugConfig
)

type HelmController struct {
	Kubeconfig      string `short:"k" usage:"Kubernetes config files, e.g. $HOME/.kube/config. May be set via KUBECONFIG env var." env:"KUBECONFIG"`
	MasterURL       string `short:"m" usage:"Kubernetes cluster master URL. May be set via MASTERURL env var." env:"MASTERURL"`
	Namespace       string `short:"n" usage:"Namespace to watch, empty means it will watch CRDs in all namespaces. May be set via NAMESPACE env var." env:"NAMESPACE"`
	Threads         int    `short:"t" usage:"Threadiness level to set. May be set via THREADS env var." default:"2" env:"THREADS"`
	ControllerName  string `usage:"Unique name to identify this controller that is added to all HelmCharts tracked by this controller. May be set via CONTROLLER_NAME env var." default:"helm-controller" env:"CONTROLLER_NAME"`
	NodeName        string `usage:"Name of the node this controller is running on. May be set via NODE_NAME env var." env:"NODE_NAME"`
	JobClusterRole  string `usage:"Name of the cluster role to use for jobs created to manage helm charts. May be set via JOB_CLUSTER_ROLE env var." default:"cluster-admin" env:"JOB_CLUSTER_ROLE"`
	DefaultJobImage string `usage:"Default image to use by jobs managing helm charts. May be set via DEFAULT_JOB_IMAGE env var." env:"DEFAULT_JOB_IMAGE"`
	PprofPort       int    `usage:"Port to publish HTTP server runtime profiling data in the format expected by the pprof visualization tool. Only enabled if in debug mode." default:"6060"`
}

func (a *HelmController) Run(cmd *cobra.Command, args []string) error {
	if debugConfig.Debug && a.PprofPort > 0 {
		go func() {
			// Serves HTTP server runtime profiling data in the format expected by the
			// pprof visualization tool at the provided endpoint on the local network
			// See https://pkg.go.dev/net/http/pprof?utm_source=gopls for more information
			log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", a.PprofPort), nil))
		}()
	}
	debugConfig.MustSetupDebug()

	cfg := a.GetNonInteractiveClientConfig()

	clientConfig, err := cfg.ClientConfig()
	if err != nil {
		return err
	}
	clientConfig.RateLimiter = ratelimit.None

	ctx := cmd.Context()
	if err := wcrd.Create(ctx, clientConfig, crd.List()); err != nil {
		return err
	}

	opts := common.Options{
		Threadiness:     a.Threads,
		NodeName:        a.NodeName,
		JobClusterRole:  a.JobClusterRole,
		DefaultJobImage: a.DefaultJobImage,
	}

	if err := opts.Validate(); err != nil {
		return err
	}

	if err := controllers.Register(ctx, a.Namespace, a.ControllerName, cfg, opts); err != nil {
		return err
	}

	<-cmd.Context().Done()
	return nil
}

func (a *HelmController) GetNonInteractiveClientConfig() clientcmd.ClientConfig {
	// Modified https://github.com/rancher/wrangler/blob/3ecd23dfea3bb4c76cbe8e06fb158eed6ae3dd31/pkg/kubeconfig/loader.go#L12-L32
	return clientcmd.NewInteractiveDeferredLoadingClientConfig(
		kubeconfig.GetLoadingRules(a.Kubeconfig),
		&clientcmd.ConfigOverrides{
			ClusterDefaults: clientcmd.ClusterDefaults,
			ClusterInfo:     clientcmdapi.Cluster{Server: a.MasterURL},
		}, nil)
}

func main() {
	cmd := command.Command(&HelmController{}, cobra.Command{
		Version: version.FriendlyVersion(),
	})
	cmd = command.AddDebug(cmd, &debugConfig)
	command.Main(cmd)
}
