package controllers

import (
	"context"
	"time"

	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	helmcontroller "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/v3/pkg/apply"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/batch"
	batchcontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	corecontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac"
	rbaccontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/rancher/wrangler/v3/pkg/leader"
	"github.com/rancher/wrangler/v3/pkg/ratelimit"
	"github.com/rancher/wrangler/v3/pkg/schemes"
	"github.com/rancher/wrangler/v3/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type appContext struct {
	helmcontroller.Interface

	K8s   kubernetes.Interface
	Core  corecontroller.Interface
	RBAC  rbaccontroller.Interface
	Batch batchcontroller.Interface

	Apply            apply.Apply
	EventBroadcaster record.EventBroadcaster

	ClientConfig clientcmd.ClientConfig
	starters     []start.Starter
}

func (a *appContext) start(ctx context.Context) error {
	return start.All(ctx, 50, a.starters...)
}

func Register(ctx context.Context, systemNamespace, controllerName string, cfg clientcmd.ClientConfig, opts common.Options) error {
	appCtx, err := newContext(cfg, systemNamespace, opts)
	if err != nil {
		return err
	}

	if len(controllerName) == 0 {
		controllerName = "helm-controller"
	}

	appCtx.EventBroadcaster.StartLogging(logrus.Infof)
	appCtx.EventBroadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{
		Interface: appCtx.K8s.CoreV1().Events(systemNamespace),
	})
	recorder := appCtx.EventBroadcaster.NewRecorder(schemes.All, corev1.EventSource{
		Component: controllerName,
		Host:      opts.NodeName,
	})

	// apply custom DefaultJobImage option to Helm before starting charts controller
	if opts.DefaultJobImage != "" {
		chart.DefaultJobImage = opts.DefaultJobImage
	}

	chart.Register(ctx,
		systemNamespace,
		controllerName,
		opts.JobClusterRole,
		"6443",
		appCtx.K8s,
		appCtx.Apply,
		recorder,
		appCtx.HelmChart(),
		appCtx.HelmChart().Cache(),
		appCtx.HelmChartConfig(),
		appCtx.HelmChartConfig().Cache(),
		appCtx.Batch.Job(),
		appCtx.Batch.Job().Cache(),
		appCtx.RBAC.ClusterRoleBinding(),
		appCtx.Core.ServiceAccount(),
		appCtx.Core.ConfigMap(),
		appCtx.Core.Secret(),
		appCtx.Core.Secret().Cache(),
	)

	klog.Infof("Starting helm controller with %d threads", opts.Threadiness)
	klog.Infof("Using cluster role '%s' for jobs managing helm charts", opts.JobClusterRole)
	klog.Infof("Using default image '%s' for jobs managing helm charts", chart.DefaultJobImage)

	if len(systemNamespace) == 0 {
		systemNamespace = metav1.NamespaceSystem
		klog.Infof("Starting %s for all namespaces with lock in %s", controllerName, systemNamespace)
	} else {
		klog.Infof("Starting %s for namespace %s", controllerName, systemNamespace)
	}

	controllerLockName := controllerName + "-lock"
	leader.RunOrDie(ctx, systemNamespace, controllerLockName, appCtx.K8s, func(ctx context.Context) {
		if err := appCtx.start(ctx); err != nil {
			klog.Fatal(err)
		}
		klog.Info("All controllers have been started")
	})

	return nil
}

func controllerFactory(rest *rest.Config) (controller.SharedControllerFactory, error) {
	rateLimit := workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 60*time.Second)
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

		Apply:            apply,
		EventBroadcaster: record.NewBroadcaster(),

		ClientConfig: cfg,
		starters: []start.Starter{
			core,
			batch,
			rbac,
			helm,
		},
	}, nil
}
