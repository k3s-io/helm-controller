package controllers

import (
	"context"
	"time"

	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	v1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/generated/controllers/batch"
	batchcontrollers "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generated/controllers/rbac"
	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/ratelimit"
	"github.com/rancher/wrangler/pkg/schemes"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type appContext struct {
	v1.Interface

	K8s   kubernetes.Interface
	Core  corecontrollers.Interface
	RBAC  rbaccontrollers.Interface
	Batch batchcontrollers.Interface

	Apply         apply.Apply
	EventRecorder record.EventRecorder

	ClientConfig clientcmd.ClientConfig
	starters     []start.Starter
}

func (a *appContext) start(ctx context.Context) error {
	return start.All(ctx, 50, a.starters...)
}

func Register(ctx context.Context, systemNamespace string, cfg clientcmd.ClientConfig, opts common.Options) error {
	appCtx, err := newContext(cfg, systemNamespace, opts)
	if err != nil {
		return err
	}

	chart.Register(ctx,
		systemNamespace,
		appCtx.K8s,
		appCtx.Apply,
		appCtx.EventRecorder,
		appCtx.HelmChart(),
		appCtx.HelmChart().Cache(),
		appCtx.HelmChartConfig(),
		appCtx.HelmChartConfig().Cache(),
		appCtx.Batch.Job(),
		appCtx.Batch.Job().Cache(),
		appCtx.RBAC.ClusterRoleBinding(),
		appCtx.Core.ServiceAccount(),
		appCtx.Core.ConfigMap())

	klog.Infof("Starting helm controller with %d threads.", opts.Threadiness)

	if len(systemNamespace) == 0 {
		klog.Info("Starting helm controller with no namespace.")
	} else {
		klog.Infof("Starting helm controller in namespace: %s.", systemNamespace)
	}

	leader.RunOrDie(ctx, systemNamespace, "helm-controller-lock", appCtx.K8s, func(ctx context.Context) {
		if err := appCtx.start(ctx); err != nil {
			klog.Fatal(err)
		}
		klog.Info("All controllers have been started")
	})

	return nil
}

func controllerFactory(rest *rest.Config) (controller.SharedControllerFactory, error) {
	rateLimit := workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 60*time.Second)
	workqueue.DefaultControllerRateLimiter()
	clientFactory, err := client.NewSharedClientFactory(rest, nil)
	if err != nil {
		return nil, err
	}

	cacheFactory := cache.NewSharedCachedFactory(clientFactory, nil)
	return controller.NewSharedControllerFactory(cacheFactory, &controller.SharedControllerFactoryOptions{
		DefaultRateLimiter: rateLimit,
		DefaultWorkers:     50,
	}), nil
}

func eventRecorder(k8s *kubernetes.Clientset, nodeName string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{Interface: k8s.CoreV1().Events(metav1.NamespaceSystem)})
	eventSource := corev1.EventSource{Component: common.Name}
	eventSource.Host = nodeName
	return eventBroadcaster.NewRecorder(schemes.All, eventSource)
}

func newContext(cfg clientcmd.ClientConfig, systemNamespace string, opts common.Options) (*appContext, error) {
	client, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}
	client.RateLimiter = ratelimit.None

	apply, err := apply.NewForConfig(client)
	if err != nil {
		return nil, err
	}
	apply = apply.WithSetOwnerReference(false, false)

	k8s, err := kubernetes.NewForConfig(client)
	if err != nil {
		return nil, err
	}

	eventRecorder := eventRecorder(k8s, opts.NodeName)

	scf, err := controllerFactory(client)
	if err != nil {
		return nil, err
	}

	core, err := core.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	corev := core.Core().V1()

	batch, err := batch.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	batchv := batch.Batch().V1()

	rbac, err := rbac.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	rbacv := rbac.Rbac().V1()

	helm, err := helm.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}
	helmv := helm.Helm().V1()

	return &appContext{
		Interface: helmv,

		K8s:   k8s,
		Core:  corev,
		Batch: batchv,
		RBAC:  rbacv,

		Apply:         apply,
		EventRecorder: eventRecorder,

		ClientConfig: cfg,
		starters: []start.Starter{
			core,
			batch,
			rbac,
			helm,
		},
	}, nil
}
