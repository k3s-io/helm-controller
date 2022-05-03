package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/k3s-io/helm-controller/pkg/controllers"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/crd"
	"github.com/k3s-io/helm-controller/pkg/version"
	command "github.com/rancher/wrangler-cli"
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
	Kubeconfig     string `short:"k" usage:"Kubernetes config files, e.g. $HOME/.kube/config" env:"KUBECONFIG"`
	MasterURL      string `short:"m" usage:"Kubernetes cluster master URL" env:"MASTERURL"`
	Namespace      string `short:"n" usage:"Namespace to watch, empty means it will watch CRDs in all namespaces." env:"NAMESPACE"`
	Threads        int    `short:"t" usage:"Threadiness level to set, defaults to 2." default:"2" env:"THREADS"`
	ControllerName string `usage:"Unique name to identify this controller that is added to all HelmCharts tracked by this controller" default:"helm-controller" env:"CONTROLLER_NAME"`
	NodeName       string `usage:"Name of the node this controller is running on" env:"NODE_NAME"`
}

func (a *HelmController) Run(cmd *cobra.Command, args []string) error {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	debugConfig.MustSetupDebug()

	cfg := a.GetNonInteractiveClientConfig()

	clientConfig, err := cfg.ClientConfig()
	if err != nil {
		return err
	}
	clientConfig.RateLimiter = ratelimit.None

	ctx := cmd.Context()
	if err := crd.Create(ctx, clientConfig); err != nil {
		return err
	}

	opts := common.Options{
		Threadiness: a.Threads,
		NodeName:    a.NodeName,
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
