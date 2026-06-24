package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/k3s-io/helm-controller/pkg/config"
	"github.com/k3s-io/helm-controller/pkg/controllers"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/crds"
	"github.com/rancher/wrangler/v3/pkg/crd"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

const (
	// readyDuration time to wait for CRDs to be ready.
	readyDuration = time.Minute * 1
)

func SetupLogging(debug bool) (logr.Logger, error) {
	klog.EnableContextualLogging(true)
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	return common.NewLogrusSink(nil).AsLogr(), nil
}

func Run(ctx context.Context, hc config.CLI) error {
	if hc.Debug && hc.PprofPort > 0 {
		go func() {
			// Serves HTTP server runtime profiling data in the format expected by the
			// pprof visualization tool at the provided endpoint on the local network
			// See https://pkg.go.dev/net/http/pprof?utm_source=gopls for more information
			log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", hc.PprofPort), nil))
		}()
	}
	logger, err := SetupLogging(hc.Debug)
	if err != nil {
		return err
	}
	ctx = klog.NewContext(ctx, logger)

	cfg := getNonInteractiveClientConfig(hc)
	rest, err := cfg.ClientConfig()
	if err != nil {
		return err
	}
	client, err := clientset.NewForConfig(rest)
	if err != nil {
		return err
	}

	crds, err := crds.List()
	if err != nil {
		return err
	}

	opts, err := hc.GetControllerConfig()
	if err != nil {
		return err
	}

	if err := crd.BatchCreateCRDs(ctx, client.ApiextensionsV1().CustomResourceDefinitions(), nil, readyDuration, crds); err != nil {
		return err
	}

	if err := controllers.Register(ctx, hc.Namespace, hc.ControllerName, cfg, opts); err != nil {
		return err
	}

	<-ctx.Done()
	return ctx.Err()
}

func getNonInteractiveClientConfig(hc config.CLI) clientcmd.ClientConfig {
	// Modified https://github.com/rancher/wrangler/blob/3ecd23dfea3bb4c76cbe8e06fb158eed6ae3dd31/pkg/kubeconfig/loader.go#L12-L32
	return clientcmd.NewInteractiveDeferredLoadingClientConfig(
		kubeconfig.GetLoadingRules(hc.Kubeconfig),
		&clientcmd.ConfigOverrides{
			ClusterDefaults: clientcmd.ClusterDefaults,
			ClusterInfo:     clientcmdapi.Cluster{Server: hc.MasterURL},
		}, nil)
}
