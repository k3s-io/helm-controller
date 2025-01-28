package cli

import (
	"flag"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"

	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/k3s-io/helm-controller/pkg/controllers"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/crd"
	wcrd "github.com/rancher/wrangler/v3/pkg/crd"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/rancher/wrangler/v3/pkg/ratelimit"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type HelmController struct {
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
	PprofPort       int
}

func (hc *HelmController) SetupDebug() error {
	logging := flag.NewFlagSet("", flag.PanicOnError)
	klog.InitFlags(logging)
	if hc.Debug {
		logrus.SetLevel(logrus.DebugLevel)
		if err := logging.Parse([]string{
			fmt.Sprintf("-v=%d", hc.DebugLevel),
		}); err != nil {
			return err
		}
	} else {
		if err := logging.Parse([]string{
			"-v=0",
		}); err != nil {
			return err
		}
	}

	return nil
}

func (a *HelmController) Run(cmd *cobra.Command, args []string) error {
	if a.Debug && a.PprofPort > 0 {
		go func() {
			// Serves HTTP server runtime profiling data in the format expected by the
			// pprof visualization tool at the provided endpoint on the local network
			// See https://pkg.go.dev/net/http/pprof?utm_source=gopls for more information
			log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", a.PprofPort), nil))
		}()
	}
	err := a.SetupDebug()
	if err != nil {
		panic("failed to setup debug logging: " + err.Error())
	}

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
