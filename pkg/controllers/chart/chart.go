package chart

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/extjson"
	helmcontroller "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/remove"
	"github.com/rancher/wrangler/v3/pkg/apply"
	batchcontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	rbaccontroller "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/rancher/wrangler/v3/pkg/merr"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

const (
	ReleaseType   = "helm.sh/release.v1"
	SecretType    = "helmcharts.helm.cattle.io/values"
	CRDName       = "helmcharts.helm.cattle.io"
	ConfigCRDName = "helmchartconfigs.helm.cattle.io"

	TaintExternalCloudProvider = "node.cloudprovider.kubernetes.io/uninitialized"

	KeyConfigHash = "helmcharts.helm.cattle.io/configHash"

	AnnotationChartURL  = "helm.cattle.io/chart-url"
	AnnotationManagedBy = "helmcharts.cattle.io/managed-by"
	AnnotationUnmanaged = "helmcharts.helm.cattle.io/unmanaged"

	LabelChartName          = "helmcharts.helm.cattle.io/chart"
	LabelNodeRolePrefix     = "node-role.kubernetes.io/"
	LabelControlPlaneSuffix = "control-plane"
	LabelEtcdSuffix         = "etcd"

	FailurePolicyAbort     = "abort"
	FailurePolicyReinstall = "reinstall"
	FailurePolicyRetry     = "retry"

	chartBySecretIndex       = "helmcharts.helm.cattle.io/chart-by-secret"
	chartConfigBySecretIndex = "helmcharts.helm.cattle.io/chartconfig-by-secret"
)

var (
	commaRE              = regexp.MustCompile(`\\*,`)
	DefaultJobImage      = "rancher/klipper-helm:latest"
	JobTolerations       []corev1.Toleration
	JobResources         *corev1.ResourceRequirements
	DefaultFailurePolicy = FailurePolicyReinstall
	defaultBackOffLimit  = ptr.To(int32(1000))

	defaultPodSecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}
	defaultSecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
		ReadOnlyRootFilesystem: ptr.To(true),
	}
	defaultPriorityClassName = "system-cluster-critical"
)

type Controller struct {
	apiServerPort   string
	jobClusterRole  string
	managedBy       string
	systemNamespace string
	logger          klog.Logger
	helms           helmcontroller.HelmChartController
	helmCache       helmcontroller.HelmChartCache
	confs           helmcontroller.HelmChartConfigController
	confCache       helmcontroller.HelmChartConfigCache
	jobs            batchcontroller.JobController
	jobCache        batchcontroller.JobCache
	configMaps      configMapLister
	secrets         secretLister
	secretCache     corecontroller.SecretCache
	apply           apply.Apply
	recorder        record.EventRecorder
}

type configMapLister interface {
	List(namespace string, opts metav1.ListOptions) (*corev1.ConfigMapList, error)
}

type secretLister interface {
	List(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error)
}

func Register(
	ctx context.Context,
	systemNamespace,
	managedBy,
	jobClusterRole string,
	apiServerPort string,
	k8s kubernetes.Interface,
	apply apply.Apply,
	recorder record.EventRecorder,
	helms helmcontroller.HelmChartController,
	helmCache helmcontroller.HelmChartCache,
	confs helmcontroller.HelmChartConfigController,
	confCache helmcontroller.HelmChartConfigCache,
	jobs batchcontroller.JobController,
	jobCache batchcontroller.JobCache,
	crbs rbaccontroller.ClusterRoleBindingController,
	sas corecontroller.ServiceAccountController,
	cm corecontroller.ConfigMapController,
	s corecontroller.SecretController,
	sCache corecontroller.SecretCache) {
	c := &Controller{
		apiServerPort:   apiServerPort,
		jobClusterRole:  jobClusterRole,
		managedBy:       managedBy,
		systemNamespace: systemNamespace,
		logger:          klog.FromContext(ctx),
		helms:           helms,
		helmCache:       helmCache,
		confs:           confs,
		confCache:       confCache,
		jobs:            jobs,
		jobCache:        jobCache,
		configMaps:      cm,
		secrets:         s,
		secretCache:     sCache,
		recorder:        recorder,
	}

	c.apply = apply.
		WithCacheTypes(helms, confs, jobs, crbs, sas, cm, s).
		WithStrictCaching().
		WithReconciler(jobs.GroupVersionKind(), c.reconcileJob)

	helmCache.AddIndexer(chartBySecretIndex, chartBySecret)
	confCache.AddIndexer(chartConfigBySecretIndex, chartConfigBySecret)

	relatedresource.Watch(ctx, "resolve-helm-chart-from-helm-chart-config", c.resolveHelmChartFromHelmChartConfig, helms, confs)
	relatedresource.Watch(ctx, "resolve-helm-chart-from-secret", c.resolveHelmChartFromSecret, helms, s)
	relatedresource.Watch(ctx, "resolve-helm-chart-config-from-secret", c.resolveHelmChartConfigFromSecret, confs, s)

	// Why do we need to add the managedBy string to the generatingHandlerName?
	//
	// By default, generating handlers use the name of the controller as the set ID for the wrangler.apply operation
	// Therefore, if multiple iterations of the helm-controller are using the same set ID, they will try to overwrite each other's
	// resources since each controller will detect the other's set as resources that need to be cleaned up to apply the new set
	//
	// To resolve this, we simply prefix the provided managedBy string to the generatingHandler controller's name only to ensure that the
	// set ID specified will only target this particular controller
	generatingHandlerName := fmt.Sprintf("%s-chart-registration", managedBy)
	helmcontroller.RegisterHelmChartGeneratingHandler(ctx, helms, c.apply, "", generatingHandlerName, c.OnChange, &generic.GeneratingHandlerOptions{
		AllowClusterScoped: true,
	})

	remove.RegisterScopedOnRemoveHandler(ctx, helms, "on-helm-chart-remove",
		func(key string, obj runtime.Object) (bool, error) {
			if obj == nil {
				return false, nil
			}
			helmChart, ok := obj.(*v1.HelmChart)
			if !ok {
				return false, nil
			}
			return c.shouldManage(helmChart)
		},
		generic.FromObjectHandlerToHandler(generic.ObjectHandler[*v1.HelmChart](c.OnRemove)),
	)

	relatedresource.Watch(ctx, "resolve-helm-chart-owned-resources",
		relatedresource.OwnerResolver(true, v1.SchemeGroupVersion.String(), "HelmChart"),
		helms,
		jobs, crbs, sas, cm,
	)
}

// reconcileJob triggers recreation of the Job if the pod template spec changes.
// If the Job is too new, the operation is reenqueued.
func (c *Controller) reconcileJob(_, newObj runtime.Object) (bool, error) {
	newJob, err := objectToJob(newObj)
	if err != nil {
		return false, err
	}
	// Old object is sourced from wrangler applied annotation, not current state.
	// Instead, we use the cached object to check current conditions.
	// ref: https://github.com/rancher/wrangler/blob/v3.7.0/pkg/apply/reconcilers.go#L136-L137
	oldJob, err := c.jobCache.Get(newJob.Namespace, newJob.Name)
	if err != nil {
		return false, err
	}
	// To avoid racing, we want to avoid creating and deleting jobs faster than
	// the controller can keep up. The addition of at least one condition
	// indicates that the controller has observed and synced the job at least
	// once.
	if len(oldJob.Status.Conditions) == 0 {
		return false, errors.New("wait for job controller sync before replace")
	}
	// To avoid replacing a job while old pods are still running, we wait for
	// there to be no active or terminating pods before deleting.
	podCount := oldJob.Status.Active
	if oldJob.Status.Terminating != nil {
		podCount += *oldJob.Status.Terminating
	}
	if podCount != 0 {
		return false, errors.New("wait for pods to terminate before replace")
	}
	return false, apply.ErrReplace
}

func (c *Controller) resolveHelmChartFromHelmChartConfig(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if len(c.systemNamespace) > 0 && namespace != c.systemNamespace {
		// do nothing if it's not in the namespace this controller was registered with
		return nil, nil
	}
	// See if there is a HelmChart with the same name/namespace as this HelmChartConfig
	if conf, ok := obj.(*v1.HelmChartConfig); ok {
		chart, err := c.helmCache.Get(conf.Namespace, conf.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		}
		if chart == nil {
			return nil, nil
		}
		return []relatedresource.Key{
			{
				Name:      conf.Name,
				Namespace: conf.Namespace,
			},
		}, nil
	}
	return nil, nil
}

func (c *Controller) resolveHelmChartFromSecret(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if len(c.systemNamespace) > 0 && namespace != c.systemNamespace {
		// do nothing if it's not in the namespace this controller was registered with
		return nil, nil
	}
	// See if there are HelmCharts in the same namespace that reference this Secret
	if secret, ok := obj.(*corev1.Secret); ok {
		charts, err := c.helmCache.GetByIndex(chartBySecretIndex, secret.Namespace+"."+secret.Name)
		if err != nil {
			return nil, err
		}
		keys := make([]relatedresource.Key, len(charts))
		for i, chart := range charts {
			keys[i].Name = chart.Name
			keys[i].Namespace = chart.Namespace
		}
		return keys, nil
	}
	return nil, nil
}

func (c *Controller) resolveHelmChartConfigFromSecret(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if len(c.systemNamespace) > 0 && namespace != c.systemNamespace {
		// do nothing if it's not in the namespace this controller was registered with
		return nil, nil
	}
	// See if there are HelmChartConfigs in the same namespace that reference this Secret
	if secret, ok := obj.(*corev1.Secret); ok {
		confs, err := c.confCache.GetByIndex(chartConfigBySecretIndex, secret.Namespace+"."+secret.Name)
		if err != nil {
			return nil, err
		}
		keys := make([]relatedresource.Key, len(confs))
		for i, conf := range confs {
			keys[i].Name = conf.Name
			keys[i].Namespace = conf.Namespace
		}
		return keys, nil
	}
	return nil, nil
}

func (c *Controller) OnChange(chart *v1.HelmChart, chartStatus v1.HelmChartStatus) ([]runtime.Object, v1.HelmChartStatus, error) {
	if shouldManage, err := c.shouldManage(chart); err != nil {
		return nil, chartStatus, err
	} else if !shouldManage {
		return nil, chartStatus, nil
	}

	if chart.DeletionTimestamp != nil {
		// this should only be called if the chart is being deleted
		return nil, chartStatus, nil
	}

	switch chart.Spec.HelmVersion {
	case "", "v3":
	default:
		c.recorder.Eventf(chart, corev1.EventTypeWarning, "UnsupportedVersion", "Unsupported Helm version %s: only v3 charts are supported", chart.Spec.HelmVersion)
		chartStatus.Conditions = []v1.HelmChartCondition{
			{
				Type:   v1.HelmChartJobCreated,
				Status: corev1.ConditionFalse,
			},
			{
				Type:    v1.HelmChartFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "Unsupported version",
				Message: "Only Helm v3 charts are supported",
			},
		}
		return nil, chartStatus, nil
	}

	if c.jobFailed(chart) {
		c.recorder.Eventf(chart, corev1.EventTypeWarning, "JobFailed", "Job has reached configured number of retries without succeeding")
		chartCopy := chart.DeepCopy()
		chartCopy.Status.Conditions = []v1.HelmChartCondition{
			{
				Type:    v1.HelmChartJobCreated,
				Status:  corev1.ConditionTrue,
				Reason:  "Job created",
				Message: fmt.Sprintf("Applying HelmChart using Job %s/%s", chart.Namespace, jobName(chart)),
			},
			{
				Type:    v1.HelmChartFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "Job failed",
				Message: "Job has reached configured number of retries without succeeding",
			},
		}
		if _, err := c.helms.UpdateStatus(chartCopy); err != nil {
			return nil, chartStatus, fmt.Errorf("unable to update status of helm chart to set failed condition: %w", err)
		}
	}

	// Jobs are created as suspended. Once the job controller syncs it and adds the
	// Suspended condition, we know that is has been observed by the job controller,
	// and we can safely manage it without triggering race conditions. If the job is
	// ready (has the Suspended condition), resume the job, and return ErrSkip. This
	// handler will be run again when the job controller updates the job to mark the
	// job as resumed.
	if c.jobReady(chart) {
		if err := c.setJobSuspended(chart, false); err != nil {
			return nil, chartStatus, fmt.Errorf("failed to resume job: %w", err)
		}
		return nil, chartStatus, generic.ErrSkip
	}

	// getJobAndRelatedResources may return ErrSkip if no changes are necessary for the job,
	// in which case the chartStatus does not get updated and no resources are modified.
	job, objs, err := c.getJobAndRelatedResources(chart)
	if err != nil {
		chartStatus.Conditions = []v1.HelmChartCondition{
			{
				Type:   v1.HelmChartJobCreated,
				Status: corev1.ConditionFalse,
			},
			{
				Type:    v1.HelmChartFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "Job create failed",
				Message: fmt.Sprintf("Failed to generate Job: %v", err),
			},
		}
		return nil, chartStatus, err
	}

	// update status
	chartStatus.JobName = job.Name
	chartStatus.Conditions = []v1.HelmChartCondition{
		{
			Type:    v1.HelmChartJobCreated,
			Status:  corev1.ConditionTrue,
			Reason:  "Job created",
			Message: fmt.Sprintf("Applying HelmChart using Job %s/%s", chart.Namespace, jobName(chart)),
		},
		{
			Type:   v1.HelmChartFailed,
			Status: corev1.ConditionFalse,
		},
	}

	// Suspend the current job before apply attempts to delete and recreate it.
	// The job may not exist, or may have already finished, or already be suspend, so
	// we don't care about whether or not this succeeds.
	_ = c.setJobSuspended(chart, true)

	// emit an event to indicate that this Helm chart is being applied
	annotations := map[string]string{KeyConfigHash: job.Spec.Template.ObjectMeta.Annotations[KeyConfigHash]}
	c.recorder.AnnotatedEventf(chart, annotations, corev1.EventTypeNormal, "ApplyJob", "Applying HelmChart from %s using Job %s/%s ", chartSource(chart), job.Namespace, job.Name)

	return append(objs, job), chartStatus, nil
}

func (c *Controller) OnRemove(key string, chart *v1.HelmChart) (*v1.HelmChart, error) {
	if shouldManage, err := c.shouldManage(chart); err != nil {
		return nil, err
	} else if !shouldManage {
		return nil, nil
	}

	switch chart.Spec.HelmVersion {
	case "", "v3":
	default:
		// do not try to uninstall unsupported chart versions
		return nil, nil
	}

	// If the job is ready (has the Suspended condition and has never been
	// started), resume the job, and return ErrSkip. This handler will be run
	// again when the job controller updates the job to mark the job as resumed.
	if c.jobReady(chart) {
		if err := c.setJobSuspended(chart, false); err != nil {
			return nil, fmt.Errorf("failed to resume job: %w", err)
		}
		return nil, generic.ErrSkip
	}

	if c.jobComplete(chart) {
		// uninstall job has successfully finished!
		c.recorder.Eventf(chart, corev1.EventTypeNormal, "RemoveJob", "Uninstalled HelmChart using Job %s/%s, removing resources", chart.Namespace, jobName(chart))

		// note: an empty apply removes all resources owned by this chart
		err := generic.ConfigureApplyForObject(c.apply, chart, &generic.GeneratingHandlerOptions{
			AllowClusterScoped: true,
		}).
			WithOwner(chart).
			WithSetID("helm-chart-registration").
			ApplyObjects()
		if err != nil {
			return nil, fmt.Errorf("unable to remove resources tied to HelmChart %s/%s: %s", chart.Namespace, chart.Name, err)
		}

		return nil, nil
	}

	// getJobAndRelatedResources will return ErrSkip if no changes are necessary for the job
	job, objs, err := c.getJobAndRelatedResources(chart)
	if err != nil {
		return nil, err
	}

	// Suspend the current job before apply attempts to delete and recreate it.
	// The job may not exist, or may have already finished, or already be suspend, so
	// we don't care about whether or not this succeeds.
	_ = c.setJobSuspended(chart, true)

	c.recorder.Eventf(chart, corev1.EventTypeNormal, "RemoveJob", "Uninstalling HelmChart using Job %s/%s ", job.Namespace, job.Name)

	if chart.Status.JobName != job.Name {
		chartCopy := chart.DeepCopy()
		chartCopy.Status.JobName = job.Name
		chart, err = c.helms.UpdateStatus(chartCopy)
		if err != nil {
			return chart, fmt.Errorf("unable to update status of helm chart to add uninstall job name %s: %w", chartCopy.Status.JobName, err)
		}
	}

	err = generic.ConfigureApplyForObject(c.apply, chart, &generic.GeneratingHandlerOptions{
		AllowClusterScoped: true,
	}).
		WithOwner(chart).
		WithSetID("helm-chart-registration").
		ApplyObjects(append(objs, job)...)
	if err != nil {
		// if err was caused by namespace termination, skip to not block indefinitely
		// namespace deletion will remove the chart
		var merrs merr.Errors
		if errors.As(err, &merrs) {
			// if err is merr.Errors, we need to check entries one by one
			for _, e := range merrs {
				if apierrors.IsForbidden(e) && apierrors.HasStatusCause(e, corev1.NamespaceTerminatingCause) {
					return nil, nil
				}
			}
		}
		if apierrors.IsForbidden(err) && apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
			return nil, nil
		}
		return nil, err
	}

	return chart, generic.ErrSkip
}

func (c *Controller) shouldManage(chart *v1.HelmChart) (bool, error) {
	if chart == nil {
		return false, nil
	}
	if len(c.systemNamespace) > 0 && chart.Namespace != c.systemNamespace {
		// do nothing if it's not in the namespace this controller was registered with
		return false, nil
	}
	if chart.Spec.Chart == "" && chart.Spec.ChartContent == "" {
		return false, nil
	}
	if chart.Annotations != nil {
		if _, ok := chart.Annotations[AnnotationUnmanaged]; ok {
			return false, nil
		}
		managedBy, ok := chart.Annotations[AnnotationManagedBy]
		if ok {
			// if the label exists, only handle this if the managedBy label matches that of this controller
			return managedBy == c.managedBy, nil
		}
	}
	// The managedBy label does not exist, so we trigger claiming the HelmChart
	// We then return false since this update will automatically retrigger an OnChange operation
	chartCopy := chart.DeepCopy()
	if chartCopy.Annotations == nil {
		chartCopy.SetAnnotations(map[string]string{
			AnnotationManagedBy: c.managedBy,
		})
	} else {
		chartCopy.Annotations[AnnotationManagedBy] = c.managedBy
	}
	_, err := c.helms.Update(chartCopy)
	return false, err
}

func (c *Controller) getJobAndRelatedResources(chart *v1.HelmChart) (*batch.Job, []runtime.Object, error) {
	// set a default failure policy
	failurePolicy := DefaultFailurePolicy
	if fp := string(chart.Spec.FailurePolicy); fp != "" {
		failurePolicy = fp
	}

	// set default for SSA force-conflicts
	forceConflicts := chart.Spec.ForceConflicts

	// override default backOffLimit if specified
	backOffLimit := defaultBackOffLimit
	if chart.Spec.BackOffLimit != nil {
		backOffLimit = chart.Spec.BackOffLimit
	}

	// get the default job and configmaps
	objects := []metav1.Object{}
	job, valuesSecret, contentConfigMap := job(chart, c.apiServerPort)

	if chart.DeletionTimestamp == nil {
		// only need content and values secrets if the chart is being installed or upgraded
		objects = append(objects, contentConfigMap, valuesSecret)

		// make sure that changes to HelmChart ValuesSecrets triger change to hash
		for _, secret := range chart.Spec.ValuesSecrets {
			if !secret.IgnoreUpdates && secret.Name != "chart-values-"+chart.Name {
				if s, err := c.secretCache.Get(chart.Namespace, secret.Name); err == nil {
					objects = append(objects, s)
				}
			}
		}

		// check if a HelmChartConfig is registered for this Helm chart
		config, err := c.confCache.Get(chart.Namespace, chart.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
		if config != nil {
			// Merge the values into the HelmChart's values
			valuesSecretAddConfig(job, valuesSecret, config)

			// Override the failure policy to what is provided in the HelmChartConfig
			if fp := string(config.Spec.FailurePolicy); fp != "" {
				failurePolicy = fp
			}

			// Override the force-conflict setting to what is provided in the HelmChartConfig
			if config.Spec.ForceConflicts != nil {
				forceConflicts = *config.Spec.ForceConflicts
			}

			// make sure that changes to HelmChart ValuesSecrets triger change to hash
			for _, secret := range config.Spec.ValuesSecrets {
				if !secret.IgnoreUpdates && secret.Name != "chart-values-"+config.Name {
					if s, err := c.secretCache.Get(chart.Namespace, secret.Name); err == nil {
						objects = append(objects, s)
					}
				}
			}
		}
	}

	// set the failure policy and add additional annotations to the job
	// note: the purpose of the additional annotation is to cause the job to be destroyed
	// and recreated if the hash of the HelmChartConfig changes while it is being processed
	setFailurePolicy(job, failurePolicy)
	setForceConflicts(job, forceConflicts)
	setBackOffLimit(job, backOffLimit)
	hashObjects(job, objects...)

	_, configHash, _ := strings.Cut(job.Spec.Template.ObjectMeta.Annotations[KeyConfigHash], "=")
	if len(configHash) > 63 { // max label value
		configHash = configHash[:63]
	}

	// get current release info
	release, err := c.getChartRelease(chart)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, fmt.Errorf("failed to get latest chart release revision: %w", err)
	}
	c.logger.V(1).Info("Resolved latest chart release",
		"chart.name", fmt.Sprintf("%s/%s", chart.Namespace, chart.Name),
		"chart.hash", configHash,
		"release.revision", release.revision,
		"release.hash", release.hash,
		"release.status", release.status,
	)

	if c.jobComplete(chart) {
		if chart.DeletionTimestamp == nil {
			// if the install or upgrade job is complete and the latest release's hash
			// label matches the config hash, then there is nothing to be done.
			if release.status == "deployed" && release.hash == configHash {
				c.logger.V(1).Info("Job is completed and deployed chart release has correct hash",
					"chart.name", fmt.Sprintf("%s/%s", chart.Namespace, chart.Name),
				)
				return job, nil, generic.ErrSkip
			}
			c.logger.V(1).Info("Job is completed but release status or hash is incorrect",
				"chart.name", fmt.Sprintf("%s/%s", chart.Namespace, chart.Name),
			)
		} else {
			// if the delete job is complete and there is no release present,
			// then there is nothing to be done
			if release.revision == 0 {
				c.logger.V(1).Info("Job is completed and no release is present",
					"chart.name", fmt.Sprintf("%s/%s", chart.Namespace, chart.Name),
				)
				return job, nil, generic.ErrSkip
			}
			c.logger.V(1).Info("Job is completed but release is still present",
				"chart.name", fmt.Sprintf("%s/%s", chart.Namespace, chart.Name),
			)
		}
	} else {
		// job is not complete, do not modify the job if the template has not changed
		if oldJob, err := c.jobCache.Get(job.Namespace, job.Name); err == nil && !templateChanged(oldJob, job) {
			return job, nil, generic.ErrSkip
		}
	}

	// inject the current chart release and hash into the job env; the helm job pod is
	// expected to validate the hash, and not take action if it does not match. If the job
	// pod does take action, the expected hash is added to the helm release resource as a label.
	// Note that this is injected AFTER the hash is calculated, so that the hash does not
	// include the revision or hash itself, which would cause an endless reconcile loop.
	for i := range job.Spec.Template.Spec.Containers {
		job.Spec.Template.Spec.Containers[i].Env = append(
			job.Spec.Template.Spec.Containers[i].Env,
			corev1.EnvVar{Name: "EXPECTED_RELEASE_REVISION", Value: strconv.FormatInt(release.revision, 10)},
			corev1.EnvVar{Name: "CONFIG_HASH", Value: configHash},
		)
	}

	return job, []runtime.Object{
		valuesSecret,
		contentConfigMap,
		serviceAccount(chart),
		roleBinding(chart, c.jobClusterRole),
	}, nil
}

func (c *Controller) getChartRelease(chart *v1.HelmChart) (release, error) {
	ls := labels.Set{"owner": "helm", "name": chart.Name}.AsSelector()

	if helmDriver(chart) == "configmap" {
		cmList, err := c.configMaps.List(chart.Spec.TargetNamespace, metav1.ListOptions{LabelSelector: ls.String()})
		if err != nil {
			return release{}, err
		}
		objects := make([]metav1.ObjectMeta, len(cmList.Items))
		for i := range cmList.Items {
			objects[i] = cmList.Items[i].ObjectMeta
		}
		return latestRelease(objects)
	}

	fs := fields.OneTermEqualSelector("type", ReleaseType)
	secretList, err := c.secrets.List(chart.Spec.TargetNamespace, metav1.ListOptions{FieldSelector: fs.String(), LabelSelector: ls.String()})
	if err != nil {
		return release{}, err
	}
	objects := make([]metav1.ObjectMeta, len(secretList.Items))
	for i := range secretList.Items {
		objects[i] = secretList.Items[i].ObjectMeta
	}
	return latestRelease(objects)
}

// jobComplete returns true if the job controller has added a True Completed
// condition to the job for the given chart.
func (c *Controller) jobComplete(chart *v1.HelmChart) bool {
	if job, _ := c.jobs.Cache().Get(chart.Namespace, jobName(chart)); job != nil {
		for _, condition := range job.Status.Conditions {
			if condition.Type == batch.JobComplete {
				return condition.Status == corev1.ConditionTrue
			}
		}
	}
	return false
}

// jobFailed returns true if the job controller has added a True Failed
// condition to the job for the given chart.
func (c *Controller) jobFailed(chart *v1.HelmChart) bool {
	if job, _ := c.jobs.Cache().Get(chart.Namespace, jobName(chart)); job != nil {
		for _, condition := range job.Status.Conditions {
			if condition.Type == batch.JobFailed {
				return condition.Status == corev1.ConditionTrue
			}
		}
	}
	return false
}

// jobReady returns true if the job is suspended, has never been started, and
// the Job controller has added a True Suspended condition to the job for the
// given chart. The addition of this condition indicates that the controller
// has observed and synced the job at least once.
// We create jobs suspended, and look for generation == 1 to indicate that the
// job has not potentially previously run; we cannot look at status.startTime as
// this is cleared when the job is suspended.
func (c *Controller) jobReady(chart *v1.HelmChart) bool {
	job, _ := c.jobs.Cache().Get(chart.Namespace, jobName(chart))
	if job != nil && job.Generation == 1 && job.Spec.Suspend != nil && *job.Spec.Suspend {
		for _, condition := range job.Status.Conditions {
			if condition.Type == batch.JobSuspended {
				return condition.Status == corev1.ConditionTrue
			}
		}
	}
	return false
}

// setJobSuspended patches the job for the given chart to set spec.suspend.
// We use a JSONPatch that tests that the value is not already set to the
// desired state, so the Patch will return an error if the change is a no-op or
// the job does not exist, which will prevent spurious events from being
// emitted.
func (c *Controller) setJobSuspended(chart *v1.HelmChart, suspend bool) error {
	name := jobName(chart)
	b := fmt.Appendf(nil, `[{"op":"test","path":"/spec/suspend","value":%t},{"op":"replace","path":"/spec/suspend","value":%t}]`, !suspend, suspend)
	_, err := c.jobs.Patch(chart.Namespace, name, types.JSONPatchType, b)
	if err == nil {
		if suspend {
			c.recorder.Eventf(chart, corev1.EventTypeNormal, "SuspendJob", "Suspended Job %s/%s for delete", chart.Namespace, name)
		} else {
			c.recorder.Eventf(chart, corev1.EventTypeNormal, "ResumeJob", "Resumed synced Job %s/%s", chart.Namespace, name)
		}
	}
	return err
}

type release struct {
	revision int64
	hash     string
	status   string
}

// latestRelease returns info for the release with the highest version, from the provided list of objects.
func latestRelease(objects []metav1.ObjectMeta) (release, error) {
	rel := release{}
	for _, obj := range objects {
		if sv, err := strconv.ParseInt(obj.Labels["version"], 10, 64); err == nil && sv > rel.revision {
			rel.revision = sv
			rel.status = obj.Labels["status"]
			rel.hash = obj.Labels[KeyConfigHash]
		}
	}
	return rel, nil
}

func chartBySecret(chart *v1.HelmChart) ([]string, error) {
	keys := sets.Set[string]{}
	for _, secret := range chart.Spec.ValuesSecrets {
		if !secret.IgnoreUpdates {
			keys.Insert(chart.Namespace + "." + secret.Name)
		}
	}
	return keys.UnsortedList(), nil
}

func chartConfigBySecret(conf *v1.HelmChartConfig) ([]string, error) {
	keys := sets.Set[string]{}
	for _, secret := range conf.Spec.ValuesSecrets {
		if !secret.IgnoreUpdates {
			keys.Insert(conf.Namespace + "." + secret.Name)
		}
	}
	return keys.UnsortedList(), nil
}

func job(chart *v1.HelmChart, apiServerPort string) (*batch.Job, *corev1.Secret, *corev1.ConfigMap) {
	jobImage := strings.TrimSpace(chart.Spec.JobImage)
	if jobImage == "" {
		jobImage = DefaultJobImage
	}

	targetNamespace := chart.Namespace
	if len(chart.Spec.TargetNamespace) != 0 {
		targetNamespace = chart.Spec.TargetNamespace
	}

	chartName := chart.Spec.Chart
	if chart.Spec.Repo != "" {
		chartName = chart.Name + "/" + chart.Spec.Chart
	}

	podSecurityContext := defaultPodSecurityContext.DeepCopy()
	securityContext := defaultSecurityContext.DeepCopy()

	job := &batch.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName(chart),
			Namespace: chart.Namespace,
			Labels: map[string]string{
				LabelChartName: chart.Name,
			},
		},
		Spec: batch.JobSpec{
			Suspend: ptr.To(true),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					Labels: map[string]string{
						LabelChartName: chart.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "helm",
							Image:           jobImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            args(chart),
							Env: []corev1.EnvVar{
								{
									Name:  "NAME",
									Value: chart.Name,
								},
								{
									Name:  "VERSION",
									Value: chart.Spec.Version,
								},
								{
									Name:  "REPO",
									Value: chart.Spec.Repo,
								},
								{
									Name:  "HELM_DRIVER",
									Value: helmDriver(chart),
								},
								{
									Name:  "CHART_NAMESPACE",
									Value: chart.Namespace,
								},
								{
									Name:  "CHART",
									Value: chartName,
								},
								{
									Name:  "HELM_VERSION",
									Value: chart.Spec.HelmVersion,
								},
								{
									Name:  "TARGET_NAMESPACE",
									Value: targetNamespace,
								},
								{
									Name:  "AUTH_PASS_CREDENTIALS",
									Value: fmt.Sprintf("%t", chart.Spec.AuthPassCredentials),
								},
								{
									Name:  "INSECURE_SKIP_TLS_VERIFY",
									Value: fmt.Sprintf("%t", chart.Spec.InsecureSkipTLSVerify),
								},
								{
									Name:  "PLAIN_HTTP",
									Value: fmt.Sprintf("%t", chart.Spec.PlainHTTP),
								},
							},
							SecurityContext: securityContext,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "klipper-helm",
									MountPath: "/home/klipper-helm/.helm",
								},
								{
									Name:      "klipper-cache",
									MountPath: "/home/klipper-helm/.cache",
								},
								{
									Name:      "klipper-config",
									MountPath: "/home/klipper-helm/.config",
								},
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
						},
					},
					ServiceAccountName: fmt.Sprintf("helm-%s", chart.Name),
					SecurityContext:    podSecurityContext,
					PriorityClassName:  defaultPriorityClassName,
					Volumes: []corev1.Volume{
						{
							Name: "klipper-helm",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "Memory",
								},
							},
						},
						{
							Name: "klipper-cache",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "Memory",
								},
							},
						},
						{
							Name: "klipper-config",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "Memory",
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "Memory",
								},
							},
						},
					},
				},
			},
		},
	}

	if chart.Spec.Timeout != nil {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "TIMEOUT",
			Value: chart.Spec.Timeout.Duration.String(),
		})
	}

	job.Spec.Template.Spec.NodeSelector = make(map[string]string)
	job.Spec.Template.Spec.NodeSelector[corev1.LabelOSStable] = "linux"

	if chart.Spec.Bootstrap {
		job.Spec.Template.Spec.NodeSelector[LabelNodeRolePrefix+LabelControlPlaneSuffix] = "true"
		job.Spec.Template.Spec.HostNetwork = true
		job.Spec.Template.Spec.Tolerations = []corev1.Toleration{
			{
				Key:    corev1.TaintNodeNotReady,
				Effect: corev1.TaintEffectNoSchedule,
			},
			{
				Key:    corev1.TaintNodeNetworkUnavailable,
				Effect: corev1.TaintEffectNoSchedule,
			},
			{
				Key:      TaintExternalCloudProvider,
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			{
				Key:      "CriticalAddonsOnly",
				Operator: corev1.TolerationOpExists,
			},
			{
				Key:      LabelNodeRolePrefix + LabelEtcdSuffix,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoExecute,
			},
			{
				Key:      LabelNodeRolePrefix + LabelControlPlaneSuffix,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, []corev1.EnvVar{
			{
				Name:  "KUBERNETES_SERVICE_HOST",
				Value: "127.0.0.1"},
			{
				Name:  "KUBERNETES_SERVICE_PORT",
				Value: apiServerPort},
			{
				Name:  "BOOTSTRAP",
				Value: "true"},
		}...)
	}

	setProxyEnv(job)
	setAuthSecret(job, chart)
	setDockerRegistrySecret(job, chart)
	setRepoCAConfigMap(job, chart)
	setPodResources(job)
	setSecurityContext(job, chart)
	setTolerations(job)

	if chart.DeletionTimestamp == nil {
		// only need content and values secrets if the chart is being installed or upgraded
		valuesSecret := setValuesSecret(job, chart)
		contentConfigMap := setContentConfigMap(job, chart)
		return job, valuesSecret, contentConfigMap
	}

	return job, nil, nil
}

func valuesSecret(chart *v1.HelmChart) *corev1.Secret {
	var secret = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("chart-values-%s", chart.Name),
			Namespace: chart.Namespace,
		},
		Type: SecretType,
		Data: map[string][]byte{},
	}

	if chart.Spec.ValuesContent != "" {
		secret.Data["HelmChartValuesContent"] = []byte(chart.Spec.ValuesContent)
	}
	if !extjson.IsEmpty(chart.Spec.Values) {
		secret.Data["HelmChartValues"] = []byte(extjson.TryToYAML(chart.Spec.Values))
	}
	if chart.Spec.RepoCA != "" {
		secret.Data["RepoCA"] = []byte(chart.Spec.RepoCA)
	}

	return secret
}

func valuesSecretAddConfig(job *batch.Job, secret *corev1.Secret, config *v1.HelmChartConfig) {
	if config.Spec.ValuesContent != "" {
		secret.Data["HelmChartConfigValuesContent"] = []byte(config.Spec.ValuesContent)
	}
	if !extjson.IsEmpty(config.Spec.Values) {
		secret.Data["HelmChartConfigValues"] = []byte(extjson.TryToYAML(config.Spec.Values))
	}

	// modify projected volumes to hold collected secret keys
	for i := range job.Spec.Template.Spec.Volumes {
		if job.Spec.Template.Spec.Volumes[i].Name != "values" {
			continue
		}
		valuesVolume := &job.Spec.Template.Spec.Volumes[i]

		// the first source in this volume is always the managed secret for this HelmChart
		// add item for HelmChartConfig ValuesContent
		if config.Spec.ValuesContent != "" {
			valuesVolume.Projected.Sources[0].Secret.Items = append(valuesVolume.Projected.Sources[0].Secret.Items, corev1.KeyToPath{Key: "HelmChartConfigValuesContent", Path: "values-1-000-HelmChartConfig-ValuesContent.yaml"})
		}
		// add item for HelmChartConfig Values
		if !extjson.IsEmpty(config.Spec.Values) {
			valuesVolume.Projected.Sources[0].Secret.Items = append(valuesVolume.Projected.Sources[0].Secret.Items, corev1.KeyToPath{Key: "HelmChartConfigValues", Path: "values-1-001-HelmChartConfig-Values.yaml"})
		}

		items := 1
		// add projection and items for HelmChartConfig ValuesSecrets
		for _, secret := range config.Spec.ValuesSecrets {
			if len(secret.Keys) == 0 || secret.Name == "chart-values-"+config.Name {
				continue
			}
			volumeProjection := corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					Optional: ptr.To(secret.IgnoreUpdates),
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
				},
			}
			for _, key := range secret.Keys {
				items++
				volumeProjection.Secret.Items = append(volumeProjection.Secret.Items, corev1.KeyToPath{Key: key, Path: fmt.Sprintf("values-1-%03d-HelmChartConfig-ValuesSecret.yaml", items)})
			}
			valuesVolume.VolumeSource.Projected.Sources = append(valuesVolume.VolumeSource.Projected.Sources, volumeProjection)
		}
	}
}

func roleBinding(chart *v1.HelmChart, jobClusterRole string) *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("helm-%s-%s", chart.Namespace, chart.Name),
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     jobClusterRole,
		},
		Subjects: []rbac.Subject{
			{
				Name:      fmt.Sprintf("helm-%s", chart.Name),
				Kind:      "ServiceAccount",
				Namespace: chart.Namespace,
			},
		},
	}
}

func serviceAccount(chart *v1.HelmChart) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("helm-%s", chart.Name),
			Namespace: chart.Namespace,
		},
		AutomountServiceAccountToken: ptr.To(true),
	}
}

func args(chart *v1.HelmChart) []string {
	if chart.DeletionTimestamp != nil {
		return []string{
			"delete",
		}
	}

	spec := chart.Spec
	args := []string{
		"install",
	}
	if spec.TargetNamespace != "" {
		args = append(args, "--namespace", spec.TargetNamespace)
	}

	if spec.CreateNamespace {
		args = append(args, "--create-namespace")
	}

	if spec.TakeOwnership {
		args = append(args, "--take-ownership")
	}

	if spec.Version != "" {
		args = append(args, "--version", spec.Version)
	}

	for _, k := range keys(spec.Set) {
		val := spec.Set[k]
		if typedVal(val) {
			args = append(args, "--set", fmt.Sprintf("%s=%s", k, val.String()))
		} else {
			args = append(args, "--set-string", fmt.Sprintf("%s=%s", k, commaRE.ReplaceAllStringFunc(val.String(), escapeComma)))
		}
	}

	return args
}

func keys(val map[string]intstr.IntOrString) []string {
	var keys []string
	for k := range val {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// typedVal is a modified version of helm's typedVal function that operates on kubernetes IntOrString types.
// Things that look like an integer, boolean, or null should use --set; everything else should use --set-string.
// Ref: https://github.com/helm/helm/blob/v3.5.4/pkg/strvals/parser.go#L415
func typedVal(val intstr.IntOrString) bool {
	if intstr.Int == val.Type {
		return true
	}
	switch strings.ToLower(val.StrVal) {
	case "true", "false", "null":
		return true
	default:
		return false
	}
}

// escapeComma should be passed a string consisting of zero or more backslashes, followed by a comma.
// If there are an even number of characters (such as `\,` or `\\\,`) then the comma is escaped.
// If there are an uneven number of characters (such as `,` or `\\,` then the comma is not escaped,
// and we need to escape it by adding an additional backslash.
// This logic is difficult if not impossible to accomplish with a simple regex submatch replace.
func escapeComma(match string) string {
	if len(match)%2 == 1 {
		match = `\` + match
	}
	return match
}

func setProxyEnv(job *batch.Job) {
	proxySysEnv := []string{
		"all_proxy",
		"ALL_PROXY",
		"http_proxy",
		"HTTP_PROXY",
		"https_proxy",
		"HTTPS_PROXY",
		"no_proxy",
		"NO_PROXY",
	}
	for _, proxyEnv := range proxySysEnv {
		proxyEnvValue := os.Getenv(proxyEnv)
		if len(proxyEnvValue) == 0 {
			continue
		}
		envar := corev1.EnvVar{
			Name:  proxyEnv,
			Value: proxyEnvValue,
		}
		job.Spec.Template.Spec.Containers[0].Env = append(
			job.Spec.Template.Spec.Containers[0].Env,
			envar)
	}
}

func contentConfigMap(chart *v1.HelmChart) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("chart-content-%s", chart.Name),
			Namespace: chart.Namespace,
		},
		Data: map[string]string{},
	}

	if chart.Spec.ChartContent != "" {
		key := fmt.Sprintf("%s.tgz.base64", chart.Name)
		configMap.Data[key] = chart.Spec.ChartContent
	}

	return configMap
}

// setValuesSecret adds a volume and volume mount to the job spec,
// and returns a secret spec containing data from the chart.
func setValuesSecret(job *batch.Job, chart *v1.HelmChart) *corev1.Secret {
	secret := valuesSecret(chart)

	// create projected volume to hold collected secret keys
	valuesVolume := corev1.Volume{
		Name: "values",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				DefaultMode: ptr.To(int32(0644)),
				Sources: []corev1.VolumeProjection{
					{
						Secret: &corev1.SecretProjection{
							Items: []corev1.KeyToPath{},
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.Name,
							},
						},
					},
				},
			},
		},
	}

	// add item for HelmChart ValuesContent
	if chart.Spec.ValuesContent != "" {
		valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items = append(valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items, corev1.KeyToPath{Key: "HelmChartValuesContent", Path: "values-0-000-HelmChart-ValuesContent.yaml"})
	}
	// add item for HelmChart Values
	if !extjson.IsEmpty(chart.Spec.Values) {
		valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items = append(valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items, corev1.KeyToPath{Key: "HelmChartValues", Path: "values-0-001-HelmChart-Values.yaml"})
	}
	// add item for HelmChart RepoCA
	if chart.Spec.RepoCA != "" {
		valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items = append(valuesVolume.VolumeSource.Projected.Sources[0].Secret.Items, corev1.KeyToPath{Key: "RepoCA", Path: "ca-file.pem"})
	}

	items := 1
	// add projection and items for HelmChart ValuesSecrets
	for _, secret := range chart.Spec.ValuesSecrets {
		if len(secret.Keys) == 0 || secret.Name == "chart-values-"+chart.Name {
			continue
		}
		volumeProjection := corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				Optional: ptr.To(secret.IgnoreUpdates),
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
			},
		}
		for _, key := range secret.Keys {
			items++
			volumeProjection.Secret.Items = append(volumeProjection.Secret.Items, corev1.KeyToPath{Key: key, Path: fmt.Sprintf("values-0-%03d-HelmChart-ValuesSecret.yaml", items)})
		}
		valuesVolume.VolumeSource.Projected.Sources = append(valuesVolume.VolumeSource.Projected.Sources, volumeProjection)
	}

	// add values volume and volume mount
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, valuesVolume)
	job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		MountPath: "/config",
		Name:      "values",
	})

	return secret
}

func setContentConfigMap(job *batch.Job, chart *v1.HelmChart) *corev1.ConfigMap {
	configMap := contentConfigMap(chart)
	if configMap == nil {
		return nil
	}

	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "content",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				DefaultMode: ptr.To(int32(0644)),
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMap.Name,
				},
			},
		},
	})

	job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		MountPath: "/chart",
		Name:      "content",
	})

	return configMap
}

func setAuthSecret(job *batch.Job, chart *v1.HelmChart) {
	if secret := chart.Spec.AuthSecret; secret != nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "auth",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: ptr.To(int32(0644)),
					SecretName:  secret.Name,
				},
			},
		})

		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			MountPath: "/auth",
			Name:      "auth",
		})
	}
}

func setDockerRegistrySecret(job *batch.Job, chart *v1.HelmChart) {
	if secret := chart.Spec.DockerRegistrySecret; secret != nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "dockerconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: ptr.To(int32(0644)),
					SecretName:  secret.Name,
					Items: []corev1.KeyToPath{{
						Key:  ".dockerconfigjson",
						Path: "config.json",
					}},
				},
			},
		})

		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			MountPath: "/home/klipper-helm/.docker",
			Name:      "dockerconfig",
		})
	}
}

func setRepoCAConfigMap(job *batch.Job, chart *v1.HelmChart) {
	if cm := chart.Spec.RepoCAConfigMap; cm != nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "ca-files",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode:          ptr.To(int32(0644)),
					LocalObjectReference: *cm,
				},
			},
		})

		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			MountPath: "/ca-files",
			Name:      "ca-files",
		})
	}
}

func setFailurePolicy(job *batch.Job, failurePolicy string) {
	job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "FAILURE_POLICY",
		Value: failurePolicy,
	})
}

func setForceConflicts(job *batch.Job, forceConflicts bool) {
	if forceConflicts {
		job.Spec.Template.Spec.Containers[0].Args = slices.Insert(job.Spec.Template.Spec.Containers[0].Args, 1, "--force-conflicts")
	}
}

func hashObjects(job *batch.Job, objs ...metav1.Object) {
	hash := sha256.New()
	if backoffLimit := job.Spec.BackoffLimit; backoffLimit != nil {
		hash.Write(fmt.Append(nil, *backoffLimit))
	}
	if obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&job.Spec.Template); err == nil {
		ust := unstructured.Unstructured{Object: obj}
		if b, err := ust.MarshalJSON(); err == nil {
			hash.Write(b)
		}
	}
	for _, obj := range objs {
		if uobj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj); err == nil {
			if data, _, err := unstructured.NestedStringMap(uobj, "data"); err == nil {
				keys := make([]string, 0, len(data))
				for k := range data {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					hash.Write([]byte(k))
					hash.Write([]byte(data[k]))
				}
			}
		}
	}

	job.Spec.Template.ObjectMeta.Annotations[KeyConfigHash] = fmt.Sprintf("SHA256=%X", hash.Sum(nil))
}

func setBackOffLimit(job *batch.Job, backOffLimit *int32) {
	job.Spec.BackoffLimit = backOffLimit
}

func setPodResources(job *batch.Job) {
	if JobResources != nil {
		job.Spec.Template.Spec.Containers[0].Resources = *JobResources.DeepCopy()
	}
}

func setSecurityContext(job *batch.Job, chart *v1.HelmChart) {
	if chart.Spec.PodSecurityContext != nil {
		job.Spec.Template.Spec.SecurityContext = chart.Spec.PodSecurityContext
	}

	if chart.Spec.SecurityContext != nil {
		job.Spec.Template.Spec.Containers[0].SecurityContext = chart.Spec.SecurityContext
	}
}

func setTolerations(job *batch.Job) {
	if len(JobTolerations) > 0 {
		job.Spec.Template.Spec.Tolerations = append(job.Spec.Template.Spec.Tolerations, JobTolerations...)
	}
}

// chartSource returns a string describing the source of the chart:
// chartContent, chart URL, or repo+version
func chartSource(chart *v1.HelmChart) string {
	if chart == nil {
		return "<unknown>"
	}

	if chart.Spec.ChartContent != "" {
		if url := chart.Annotations[AnnotationChartURL]; url != "" {
			return fmt.Sprintf("inline spec.chartContent from %s", url)
		}
		return "inline spec.chartContent"
	}

	if strings.HasPrefix(chart.Spec.Chart, "oci://") {
		if chart.Spec.Version != "" {
			return fmt.Sprintf("version %s from OCI registry %s", chart.Spec.Version, chart.Spec.Chart)
		}
		return fmt.Sprintf("latest stable version from OCI registry %s", chart.Spec.Chart)
	}

	if strings.Contains(chart.Spec.Chart, "://") {
		return chart.Spec.Chart
	}

	if chart.Spec.Version != "" {
		return fmt.Sprintf("%s version %s from chart repository %s", chart.Spec.Chart, chart.Spec.Version, chart.Spec.Repo)
	}

	return fmt.Sprintf("latest stable version of %s from chart repository %s", chart.Spec.Chart, chart.Spec.Repo)
}

// templateChanged returns true if the job's pod template has changed.
// Labels and fields managed by Kubernetes are ignored.
// The CONFIG_HASH and EXPECTED_RELEASE_REVISION env vars are ignored, as they are expected to change once the chart has been updated,
// and MUST NOT trigger a re-run of the pod or we will end up in a loop.
func templateChanged(oldJob, newJob *batch.Job) bool {
	oldPodTemplate := oldJob.Spec.Template.DeepCopy()
	newPodTemplate := newJob.Spec.Template.DeepCopy()
	for _, template := range []*corev1.PodTemplateSpec{oldPodTemplate, newPodTemplate} {
		template.Spec.DeprecatedServiceAccount = template.Spec.ServiceAccountName
		template.Spec.TerminationGracePeriodSeconds = nil
		template.Spec.DNSPolicy = corev1.DNSClusterFirst
		template.Spec.SchedulerName = ""
		template.Labels = nil
		for c := range template.Spec.Containers {
			template.Spec.Containers[c].TerminationMessagePath = corev1.TerminationMessagePathDefault
			template.Spec.Containers[c].TerminationMessagePolicy = corev1.TerminationMessageReadFile
			template.Spec.Containers[c].Env = slices.DeleteFunc(template.Spec.Containers[c].Env, func(e corev1.EnvVar) bool {
				switch e.Name {
				case "CONFIG_HASH", "EXPECTED_RELEASE_REVISION":
					return true
				default:
					return false
				}
			})
		}
	}
	return !equality.Semantic.DeepEqual(oldPodTemplate, newPodTemplate)
}

func jobName(chart *v1.HelmChart) string {
	action := "install"
	if chart.DeletionTimestamp != nil {
		action = "delete"
	}
	return fmt.Sprintf("helm-%s-%s", action, chart.Name)
}

func helmDriver(chart *v1.HelmChart) string {
	if chart.Spec.Driver != "" {
		return string(chart.Spec.Driver)
	}
	return "secret"
}

func objectToJob(obj runtime.Object) (*batch.Job, error) {
	if job, ok := obj.(*batch.Job); ok {
		return job, nil
	}
	uObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("expected unstructured but got %v", reflect.TypeOf(obj))
	}
	bytes, err := uObj.MarshalJSON()
	if err != nil {
		return nil, err
	}
	job := &batch.Job{}
	return job, json.Unmarshal(bytes, job)
}
