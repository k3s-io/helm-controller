package chart

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	helmcontroller "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/remove"
	"github.com/rancher/wrangler/pkg/apply"
	batchcontroller "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbaccontroller "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/relatedresource"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
)

const (
	Label         = "helmcharts.helm.cattle.io/chart"
	Annotation    = "helmcharts.helm.cattle.io/configHash"
	Unmanaged     = "helmcharts.helm.cattle.io/unmanaged"
	ManagedBy     = "helmcharts.cattle.io/managed-by"
	CRDName       = "helmcharts.helm.cattle.io"
	ConfigCRDName = "helmchartconfigs.helm.cattle.io"

	TaintExternalCloudProvider = "node.cloudprovider.kubernetes.io/uninitialized"
	LabelNodeRolePrefix        = "node-role.kubernetes.io/"
	LabelControlPlaneSuffix    = "control-plane"
	LabelEtcdSuffix            = "etcd"

	FailurePolicyReinstall = "reinstall"
	FailurePolicyAbort     = "abort"
)

var (
	commaRE              = regexp.MustCompile(`\\*,`)
	deletePolicy         = metav1.DeletePropagationForeground
	DefaultJobImage      = "rancher/klipper-helm:v0.8.2-build20230815"
	DefaultFailurePolicy = FailurePolicyReinstall
	defaultBackOffLimit  = pointer.Int32(1000)
)

type Controller struct {
	systemNamespace string
	managedBy       string
	helms           helmcontroller.HelmChartController
	helmCache       helmcontroller.HelmChartCache
	confs           helmcontroller.HelmChartConfigController
	confCache       helmcontroller.HelmChartConfigCache
	jobs            batchcontroller.JobController
	jobCache        batchcontroller.JobCache
	apply           apply.Apply
	recorder        record.EventRecorder
	apiServerPort   string
}

func Register(ctx context.Context,
	systemNamespace, managedBy, apiServerPort string,
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
	s corecontroller.SecretController) {

	c := &Controller{
		systemNamespace: systemNamespace,
		managedBy:       managedBy,
		helms:           helms,
		helmCache:       helmCache,
		confs:           confs,
		confCache:       confCache,
		jobs:            jobs,
		jobCache:        jobCache,
		recorder:        recorder,
		apiServerPort:   apiServerPort,
	}

	c.apply = apply.
		WithCacheTypes(helms, confs, jobs, crbs, sas, cm, s).
		WithStrictCaching().
		WithPatcher(jobs.GroupVersionKind(), c.jobPatcher)

	relatedresource.Watch(ctx, "resolve-helm-chart-from-config", c.resolveHelmChartFromConfig, helms, confs)

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
		helmcontroller.FromHelmChartHandlerToHandler(c.OnRemove),
	)

	relatedresource.Watch(ctx, "resolve-helm-chart-owned-resources",
		relatedresource.OwnerResolver(true, v1.SchemeGroupVersion.String(), "HelmChart"),
		helms,
		jobs, crbs, sas, cm,
	)
}

func (c *Controller) jobPatcher(namespace, name string, pt types.PatchType, data []byte) (runtime.Object, error) {
	err := c.jobs.Delete(namespace, name, &metav1.DeleteOptions{PropagationPolicy: &deletePolicy})
	if err == nil || apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("create or replace job")
	}
	return nil, err
}

func (c *Controller) resolveHelmChartFromConfig(namespace, name string, obj runtime.Object) ([]relatedresource.Key, error) {
	if len(c.systemNamespace) > 0 && namespace != c.systemNamespace {
		// do nothing if it's not in the namespace this controller was registered with
		return nil, nil
	}
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

func (c *Controller) OnChange(chart *v1.HelmChart, chartStatus v1.HelmChartStatus) ([]runtime.Object, v1.HelmChartStatus, error) {
	if shouldManage, err := c.shouldManage(chart); err != nil {
		return nil, chartStatus, err
	} else if !shouldManage {
		return nil, chartStatus, nil
	}
	if chart.DeletionTimestamp != nil {
		// this should only be called if the chart is being installed or upgraded
		return nil, chartStatus, nil
	}

	job, objs, err := c.getJobAndRelatedResources(chart)
	if err != nil {
		return nil, chartStatus, err
	}
	// update status
	chartStatus.JobName = job.Name

	// emit an event to indicate that this Helm chart is being applied
	c.recorder.Eventf(chart, corev1.EventTypeNormal, "ApplyJob", "Applying HelmChart using Job %s/%s", job.Namespace, job.Name)

	return append(objs, job), chartStatus, nil
}

func (c *Controller) OnRemove(key string, chart *v1.HelmChart) (*v1.HelmChart, error) {
	if chart == nil {
		return nil, nil
	}

	expectedJob, objs, err := c.getJobAndRelatedResources(chart)
	if err != nil {
		return nil, err
	}

	// note: on the logic of running an apply here...
	// if the uninstall job does not exist, it will create it
	// if the job already exists and it is uninstalling, nothing will change since there's no need to patch
	// if the job already exists but is tied to an install or upgrade, there will be a need to patch so
	// the apply will execute the jobPatcher, which will delete the install/upgrade job and recreate a uninstall job
	err = generic.ConfigureApplyForObject(c.apply, chart, &generic.GeneratingHandlerOptions{
		AllowClusterScoped: true,
	}).
		WithOwner(chart).
		WithSetID("helm-chart-registration").
		ApplyObjects(append(objs, expectedJob)...)
	if err != nil {
		return nil, err
	}

	// sleep for 3 seconds to give the job time to perform the helm install
	// before emitting any errors
	time.Sleep(3 * time.Second)

	// once we have run the above logic, we can now check if the job is complete
	job, err := c.jobCache.Get(chart.Namespace, expectedJob.Name)
	if apierrors.IsNotFound(err) {
		// the above apply should have created it, something is wrong.
		// if you are here, there must be a bug in the code.
		return chart, fmt.Errorf("could not perform uninstall: expected job %s/%s to exist after apply, but not found", chart.Namespace, expectedJob.Name)
	} else if err != nil {
		return chart, err
	}

	// the first time we call this, the job will definitely not be complete... however,
	// throwing an error here will re-enqueue this controller, which will process the apply again
	// and get the job object from the cache to check again
	if job.Status.Succeeded <= 0 {
		// temporarily recreate the chart, but keep the deletion timestamp
		chartCopy := chart.DeepCopy()
		chartCopy.Status.JobName = job.Name
		newChart, err := c.helms.Update(chartCopy)
		if err != nil {
			return chart, fmt.Errorf("unable to update status of helm chart to add uninstall job name %s", chartCopy.Status.JobName)
		}
		return newChart, fmt.Errorf("waiting for delete of helm chart for %s by %s", key, job.Name)
	}

	// uninstall job has successfully finished!
	c.recorder.Eventf(chart, corev1.EventTypeNormal, "RemoveJob", "Uninstalled HelmChart using Job %s/%s, removing resources", job.Namespace, job.Name)

	// note: an empty apply removes all resources owned by this chart
	err = generic.ConfigureApplyForObject(c.apply, chart, &generic.GeneratingHandlerOptions{
		AllowClusterScoped: true,
	}).
		WithOwner(chart).
		WithSetID("helm-chart-registration").
		ApplyObjects()
	if err != nil {
		return nil, fmt.Errorf("unable to remove resources tied to HelmChart %s/%s: %s", chart.Namespace, chart.Name, err)
	}

	return chart, nil
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
		if _, ok := chart.Annotations[Unmanaged]; ok {
			return false, nil
		}
		managedBy, ok := chart.Annotations[ManagedBy]
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
			ManagedBy: c.managedBy,
		})
	} else {
		chartCopy.Annotations[ManagedBy] = c.managedBy
	}
	_, err := c.helms.Update(chartCopy)
	return false, err
}

func (c *Controller) getJobAndRelatedResources(chart *v1.HelmChart) (*batch.Job, []runtime.Object, error) {
	// set a default failure policy
	failurePolicy := DefaultFailurePolicy
	if chart.Spec.FailurePolicy != "" {
		failurePolicy = chart.Spec.FailurePolicy
	}

	// override default backOffLimit if specified
	backOffLimit := defaultBackOffLimit
	if chart.Spec.BackOffLimit != nil {
		backOffLimit = chart.Spec.BackOffLimit
	}

	// get the default job and configmaps
	job, valuesSecret, contentConfigMap := job(chart, c.apiServerPort)

	// check if a HelmChartConfig is registered for this Helm chart
	config, err := c.confCache.Get(chart.Namespace, chart.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
	}
	if config != nil {
		// Merge the values into the HelmChart's values
		valuesSecretAddConfig(valuesSecret, config)
		// Override the failure policy to what is provided in the HelmChartConfig
		if config.Spec.FailurePolicy != "" {
			failurePolicy = config.Spec.FailurePolicy
		}
	}
	// set the failure policy and add additional annotations to the job
	// note: the purpose of the additional annotation is to cause the job to be destroyed
	// and recreated if the hash of the HelmChartConfig changes while it is being processed
	setFailurePolicy(job, failurePolicy)
	setBackOffLimit(job, backOffLimit)
	hashObjects(job, contentConfigMap, valuesSecret)

	return job, []runtime.Object{
		valuesSecret,
		contentConfigMap,
		serviceAccount(chart),
		roleBinding(chart),
	}, nil
}

func job(chart *v1.HelmChart, apiServerPort string) (*batch.Job, *corev1.Secret, *corev1.ConfigMap) {
	jobImage := strings.TrimSpace(chart.Spec.JobImage)
	if jobImage == "" {
		jobImage = DefaultJobImage
	}

	action := "install"
	if chart.DeletionTimestamp != nil {
		action = "delete"
	}

	targetNamespace := chart.Namespace
	if len(chart.Spec.TargetNamespace) != 0 {
		targetNamespace = chart.Spec.TargetNamespace
	}

	chartName := chart.Spec.Chart
	if chart.Spec.Repo != "" {
		chartName = chart.Name + "/" + chart.Spec.Chart
	}

	job := &batch.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("helm-%s-%s", action, chart.Name),
			Namespace: chart.Namespace,
			Labels: map[string]string{
				Label: chart.Name,
			},
		},
		Spec: batch.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					Labels: map[string]string{
						Label: chart.Name,
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
									Value: "secret",
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
							},
						},
					},
					ServiceAccountName: fmt.Sprintf("helm-%s", chart.Name),
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
	valuesSecret := setValuesSecret(job, chart)
	contentConfigMap := setContentConfigMap(job, chart)

	return job, valuesSecret, contentConfigMap
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
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{},
	}

	if chart.Spec.ValuesContent != "" {
		secret.StringData["values-01_HelmChart.yaml"] = chart.Spec.ValuesContent
	}
	if chart.Spec.RepoCA != "" {
		secret.StringData["ca-file.pem"] = chart.Spec.RepoCA
	}

	return secret
}

func valuesSecretAddConfig(secret *corev1.Secret, config *v1.HelmChartConfig) {
	if config.Spec.ValuesContent != "" {
		secret.StringData["values-10_HelmChartConfig.yaml"] = config.Spec.ValuesContent
	}
}

func roleBinding(chart *v1.HelmChart) *rbac.ClusterRoleBinding {
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
			Name:     "cluster-admin",
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
		AutomountServiceAccountToken: pointer.BoolPtr(true),
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

func setValuesSecret(job *batch.Job, chart *v1.HelmChart) *corev1.Secret {
	secret := valuesSecret(chart)

	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "values",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secret.Name,
			},
		},
	})

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
					SecretName: secret.Name,
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
					SecretName: secret.Name,
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

func hashObjects(job *batch.Job, objs ...metav1.Object) {
	hash := sha256.New()

	for _, obj := range objs {
		if uobj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj); err == nil {
			for _, field := range []string{"data", "binaryData", "stringData"} {
				if data, _, err := unstructured.NestedStringMap(uobj, field); err == nil {
					for k, v := range data {
						hash.Write([]byte(k))
						hash.Write([]byte(v))
					}
				}
			}
		}
	}

	job.Spec.Template.ObjectMeta.Annotations[Annotation] = fmt.Sprintf("SHA256=%X", hash.Sum(nil))
}

func setBackOffLimit(job *batch.Job, backOffLimit *int32) {
	job.Spec.BackoffLimit = backOffLimit
}
