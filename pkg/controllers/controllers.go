package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/k3s-io/helm-controller/pkg/config"
	"github.com/k3s-io/helm-controller/pkg/controllers/chart"
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

const (
	eventLogLevel klog.Level = 0
)

type appContext struct {
	Apply apply.Apply

	K8s   kubernetes.Interface
	Helm  helmcontroller.Interface
	Core  corecontroller.Interface
	RBAC  rbaccontroller.Interface
	Batch batchcontroller.Interface

	EventBroadcaster record.EventBroadcaster
	ClientConfig     clientcmd.ClientConfig

	starters []start.Starter
}

func (a *appContext) start(ctx context.Context) error {
	return start.All(ctx, 50, a.starters...)
}

func Register(ctx context.Context, systemNamespace, controllerName string, cfg clientcmd.ClientConfig, opts *config.Controller) error {
	if opts == nil {
		return errors.New("invalid controller config")
	}

	if len(controllerName) == 0 {
		controllerName = "helm-controller"
	}

	ctx = klog.NewContext(ctx, klog.FromContext(ctx).WithName(controllerName))
	appCtx, err := newContext(ctx, cfg, systemNamespace, opts)
	if err != nil {
		return err
	}

	appCtx.EventBroadcaster.StartStructuredLogging(eventLogLevel)
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
	chart.JobResources = opts.JobResources
	chart.JobTolerations = opts.JobTolerations

	chart.Register(ctx,
		systemNamespace,
		controllerName,
		opts.JobClusterRole,
		"6443",
		appCtx.Apply,
		recorder,
		appCtx.Batch,
		appCtx.Core,
		appCtx.Helm,
		appCtx.RBAC,
	)

	resources, _ := json.Marshal(chart.JobResources)
	logger := klog.FromContext(ctx)
	logger.Info("Starting helm controller", "threads", opts.Threadiness)
	logger.Info("Using cluster role for jobs managing helm charts", "jobClusterRole", opts.JobClusterRole)
	logger.Info("Using default image for jobs managing helm charts", "defaultJobImage", chart.DefaultJobImage)
	logger.Info("Using resource limits for jobs managing helm charts", "jobResources", string(resources))
	logger.Info("Using tolerations for jobs managing helm charts", "jobTolerationsCount", len(chart.JobTolerations))

	if len(systemNamespace) == 0 {
		systemNamespace = metav1.NamespaceSystem
		logger.Info("Starting global controller", "leaseNamespace", systemNamespace)
	} else {
		logger.Info("Starting namespaced controller", "namespace", systemNamespace)
	}

	controllerLockName := controllerName + "-lock"
	leader.RunOrDie(ctx, systemNamespace, controllerLockName, appCtx.K8s, func(ctx context.Context) {
		if err := appCtx.start(ctx); err != nil {
			panic(fmt.Errorf("failed to start controllers: %w", err))
		}
		logger.Info("All controllers have been started")
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

func newContext(ctx context.Context, cfg clientcmd.ClientConfig, systemNamespace string, opts *config.Controller) (*appContext, error) {
	client, err := cfg.ClientConfig()
	if err != nil {
		return nil, err
	}
	client.RateLimiter = ratelimit.None

	apply, err := apply.NewForConfig(client)
	if err != nil {
		return nil, err
	}
	apply = apply.WithSetOwnerReference(false, false).WithContext(ctx)

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

	batch, err := batch.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}

	rbac, err := rbac.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}

	helm, err := helm.NewFactoryFromConfigWithOptions(client, &generic.FactoryOptions{
		SharedControllerFactory: scf,
		Namespace:               systemNamespace,
	})
	if err != nil {
		return nil, err
	}

	return &appContext{
		Apply: apply,

		K8s:   k8s,
		Batch: batch.Batch().V1(),
		Core:  core.Core().V1(),
		Helm:  helm.Helm().V1(),
		RBAC:  rbac.Rbac().V1(),

		EventBroadcaster: record.NewBroadcaster(record.WithContext(ctx)),

		ClientConfig: cfg,
		starters: []start.Starter{
			core,
			batch,
			rbac,
			helm,
		},
	}, nil
}
