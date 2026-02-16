package framework

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/util/retry"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	"github.com/k3s-io/helm-controller/pkg/controllers/extjson"
	helmcrd "github.com/k3s-io/helm-controller/pkg/crds"
	helmcln "github.com/k3s-io/helm-controller/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/wrangler/v3/pkg/condition"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	conditionComplete = condition.Cond(batchv1.JobComplete)
	conditionFailed   = condition.Cond(batchv1.JobFailed)
)

type Framework struct {
	HelmClientSet *helmcln.Clientset
	ClientSet     *kubernetes.Clientset
	ClientExt     *extclient.Clientset
	crds          []*apiextv1.CustomResourceDefinition
	Kubeconfig    string
	Name          string
	Namespace     string
	PID           int
}

func New() (*Framework, error) {
	framework := &Framework{}
	ginkgo.BeforeAll(framework.BeforeAll)
	ginkgo.AfterAll(framework.AfterAll)
	return framework, nil
}

func (f *Framework) BeforeAll() {
	f.beforeFramework()
	err := f.setupController(context.TODO())
	if err != nil {
		errExit("Failed to set up helm controller", err)
	}
}

func (f *Framework) AfterAll() {
	if ginkgo.CurrentSpecReport().Failed() {
		podList, _ := f.ClientSet.CoreV1().Pods(f.Namespace).List(context.Background(), metav1.ListOptions{})
		for _, pod := range podList.Items {
			containerNames := []string{}
			for _, container := range pod.Spec.InitContainers {
				containerNames = append(containerNames, container.Name)
			}
			for _, container := range pod.Spec.Containers {
				containerNames = append(containerNames, container.Name)
			}
			for _, container := range containerNames {
				reportName := fmt.Sprintf("podlogs-%s-%s", pod.Name, container)
				logs := f.ClientSet.CoreV1().Pods(f.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container})
				if logStreamer, err := logs.Stream(context.Background()); err == nil {
					if podLogs, err := io.ReadAll(logStreamer); err == nil {
						ginkgo.AddReportEntry(reportName, string(podLogs))
					}
				}
			}
		}
	}
	if err := f.teardownController(context.TODO()); err != nil {
		errExit("Failed to teardown helm controller", err)
	}
}

func (f *Framework) beforeFramework() {
	ginkgo.By("Creating a kubernetes client")
	f.Kubeconfig = os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", f.Kubeconfig)
	errExit("Failed to build a rest config from file", err)
	helmcln, err := helmcln.NewForConfig(config)
	errExit("Failed to initiate helm client", err)
	clientset, err := kubernetes.NewForConfig(config)
	errExit("Failed to initiate a client set", err)
	clientext, err := extclient.NewForConfig(config)
	errExit("Failed to initiate a extension-apiserver client set", err)
	f.crds, err = helmcrd.List()
	errExit("Failed to construct helm crds", err)

	f.HelmClientSet = helmcln
	f.ClientSet = clientset
	f.ClientExt = clientext
	f.Name = common.Name
	f.Namespace = common.Name
}

func errExit(msg string, err error) {
	if err == nil {
		return
	}
	logrus.Panicf("%s: %v", msg, err)
}

func (f *Framework) NewHelmChart(name, chart, version, helmVersion, values, valuesContent string, set map[string]intstr.IntOrString) *v1.HelmChart {
	return &v1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.Namespace,
			Labels: map[string]string{
				"helm-test": "true",
			},
		},
		Spec: v1.HelmChartSpec{
			Chart:         chart,
			Version:       version,
			Repo:          "",
			Values:        extjson.TryFromYAML(values),
			ValuesContent: valuesContent,
			Set:           set,
			HelmVersion:   helmVersion,
		},
	}
}

func (f *Framework) NewHelmChartConfig(name, values, valuesContent string) *v1.HelmChartConfig {
	return &v1.HelmChartConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.Namespace,
			Labels: map[string]string{
				"helm-test": "true",
			},
		},
		Spec: v1.HelmChartConfigSpec{
			Values:        extjson.TryFromYAML(values),
			ValuesContent: valuesContent,
		},
	}
}

func (f *Framework) ListReleases(chart *v1.HelmChart) ([]corev1.Secret, error) {
	labelSelector := labels.SelectorFromSet(labels.Set{
		"owner": "helm",
		"name":  chart.Name,
	})
	namespace := chart.Namespace
	if chart.Spec.TargetNamespace != "" {
		namespace = chart.Spec.TargetNamespace
	}

	secretList, err := f.ClientSet.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector.String()})

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return secretList.Items, nil
}

// GetDeployedRelease fetches the secret containing meta data about the currently deployed helm release.
func (f *Framework) GetDeployedRelease(chart *v1.HelmChart) (*corev1.Secret, error) {
	labelSelector := labels.SelectorFromSet(labels.Set{
		"owner":  "helm",
		"name":   chart.Name,
		"status": "deployed",
	})
	namespace := chart.Namespace
	if chart.Spec.TargetNamespace != "" {
		namespace = chart.Spec.TargetNamespace
	}

	secretList, err := f.ClientSet.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}
	if len(secretList.Items) != 1 {
		return nil, fmt.Errorf("expected 1 deployed release, found %d", len(secretList.Items))
	}

	return &secretList.Items[0], nil
}

// GetReleaseConfig decodes the base64 encoded gzipped helm release data stored in the secret.
func (f *Framework) GetReleaseConfig(release *corev1.Secret) (map[string]any, error) {
	data, ok := release.Data["release"]
	if !ok {
		return nil, errors.New("no release data found in secret")
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	jsonBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	releaseData := make(map[string]any)
	if err = json.Unmarshal(jsonBytes, &releaseData); err != nil {
		return nil, err
	}
	config, ok := releaseData["config"].(map[string]any)
	if !ok {
		return nil, errors.New("no config found in release data")
	}
	return config, nil
}

func (f *Framework) GetDeployedReleaseConfig(chart *v1.HelmChart) (map[string]any, error) {
	release, err := f.GetDeployedRelease(chart)
	if err != nil {
		return nil, err
	}
	return f.GetReleaseConfig(release)
}

func (f *Framework) CreateHelmChart(chart *v1.HelmChart, namespace string) (*v1.HelmChart, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Create(context.TODO(), chart, metav1.CreateOptions{})
}

func (f *Framework) UpdateHelmChart(chart *v1.HelmChart, namespace string) (updated *v1.HelmChart, err error) {
	hcs := f.HelmClientSet.HelmV1()
	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated, err = hcs.HelmCharts(namespace).Get(context.TODO(), chart.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		updated.Spec = chart.Spec
		_, err = hcs.HelmCharts(namespace).Update(context.TODO(), updated, metav1.UpdateOptions{})
		return err
	}); err != nil {
		updated = nil
	}
	return updated, err
}

func (f *Framework) DeleteHelmChart(name, namespace string) error {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (f *Framework) GetHelmChart(name, namespace string) (*v1.HelmChart, error) {
	r, err := f.HelmClientSet.HelmV1().HelmCharts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (f *Framework) ListHelmCharts(labelSelector, namespace string) (*v1.HelmChartList, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (f *Framework) CreateHelmChartConfig(chartConfig *v1.HelmChartConfig, namespace string) (*v1.HelmChartConfig, error) {
	return f.HelmClientSet.HelmV1().HelmChartConfigs(namespace).Create(context.TODO(), chartConfig, metav1.CreateOptions{})
}

func (f *Framework) DeleteHelmChartConfig(name, namespace string) error {
	return f.HelmClientSet.HelmV1().HelmChartConfigs(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (f *Framework) ListChartPods(chart *v1.HelmChart, appName string) ([]corev1.Pod, error) {
	labelSelector := labels.SelectorFromSet(labels.Set{
		"app":     appName,
		"release": chart.Name,
	})

	namespace := chart.Namespace
	if chart.Spec.TargetNamespace != "" {
		namespace = chart.Spec.TargetNamespace
	}

	podList, err := f.ClientSet.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector.String()})

	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return podList.Items, nil
}

func (f *Framework) GetJob(chart *v1.HelmChart) (*batchv1.Job, error) {
	if chart.Status.JobName == "" {
		return nil, errors.New("waiting for job name to be populated")
	}
	r, err := f.ClientSet.BatchV1().Jobs(chart.Namespace).Get(context.TODO(), chart.Status.JobName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// GetChartContent returns the base64-encoded chart tarball,
// downloaded from the specified URL.
func (f *Framework) GetChartContent(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected HTTP response: %s", resp.Status)
	}

	b := &bytes.Buffer{}
	w := base64.NewEncoder(base64.StdEncoding, b)
	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return string(b.Bytes()), nil
}

// GetHelmChartCondition returns true if there is a condition on the chart matching the selected type, status, and reason
func (f *Framework) GetHelmChartCondition(chart *v1.HelmChart, condition v1.HelmChartConditionType, status corev1.ConditionStatus, reason string) bool {
	for _, v := range chart.Status.Conditions {
		if v.Type == condition && v.Status == status && v.Reason == reason {
			return true
		}
	}
	return false
}

// CreateNamespace creates a namespace with the given name. If no error occurred and activate is true, the new namespace will be activated in Framework
func (f *Framework) CreateNamespace(name string, activate bool) error {
	_, err := f.ClientSet.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	if err == nil && activate {
		f.Namespace = name
	}
	return err
}

// DeleteNamespace removes the namespace with the given name from the cluster. If deactivate is true, the active namespace of Framework will be reset to default
func (f *Framework) DeleteNamespace(name string, deactivate bool) error {
	err := f.ClientSet.CoreV1().Namespaces().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if deactivate {
		f.Namespace = common.Name
	}
	return err
}

// ListNamespaces returns a slice of namespaces from the cluster. If filterName is not empty, only matching namespaces will be returned
func (f *Framework) ListNamespaces(filterName string) ([]corev1.Namespace, error) {
	fieldSelector := ""
	if filterName != "" {
		fieldSelector = fields.SelectorFromSet(fields.Set{
			"metadata.name": filterName,
		}).String()
	}
	nsList, err := f.ClientSet.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return nsList.Items, nil
}
