package framework

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/util/retry"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/common"
	helmcrd "github.com/k3s-io/helm-controller/pkg/crd"
	helmcln "github.com/k3s-io/helm-controller/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
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
	crdFactory    *crd.Factory
	crds          []crd.CRD
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
	crdFactory, err := crd.NewFactoryFromClient(config)
	errExit("Failed initiate factory client", err)
	f.crds = helmcrd.List()
	errExit("Failed to construct helm crd", err)

	f.HelmClientSet = helmcln
	f.ClientSet = clientset
	f.crdFactory = crdFactory
	f.Name = common.Name
	f.Namespace = common.Name

}

func errExit(msg string, err error) {
	if err == nil {
		return
	}
	logrus.Panicf("%s: %v", msg, err)
}

func (f *Framework) NewHelmChart(name, chart, version, helmVersion, valuesContent string, set map[string]intstr.IntOrString) *v1.HelmChart {
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
			ValuesContent: valuesContent,
			Set:           set,
			HelmVersion:   helmVersion,
		},
	}
}

func (f *Framework) WaitForRelease(chart *v1.HelmChart, labelSelector labels.Selector, timeout time.Duration, count int) (secrets []corev1.Secret, err error) {
	namespace := chart.Namespace
	if chart.Spec.TargetNamespace != "" {
		namespace = chart.Spec.TargetNamespace
	}

	return secrets, wait.Poll(5*time.Second, timeout, func() (bool, error) {
		list, err := f.ClientSet.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			return false, err
		}
		secrets = list.Items
		return len(secrets) == count, nil
	})
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
	return
}

func (f *Framework) DeleteHelmChart(name, namespace string) error {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (f *Framework) GetHelmChart(name, namespace string) (*v1.HelmChart, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (f *Framework) ListHelmCharts(labelSelector, namespace string) (*v1.HelmChartList, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// WaitForChartApp will check the for pods created by the chart
func (f *Framework) WaitForChartApp(chart *v1.HelmChart, appName string, timeout time.Duration, count int) (pods []corev1.Pod, err error) {
	labelSelector := labels.SelectorFromSet(labels.Set{
		"app":     appName,
		"release": chart.Name,
	})

	return pods, wait.Poll(5*time.Second, timeout, func() (bool, error) {
		list, err := f.ClientSet.CoreV1().Pods(chart.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			return false, err
		}
		pods = list.Items
		return len(pods) >= count, nil
	})
}

func (f *Framework) GetJob(chart *v1.HelmChart) (*batchv1.Job, error) {
	if chart.Status.JobName == "" {
		return nil, fmt.Errorf("waiting for job name to be populated")
	}
	return f.ClientSet.BatchV1().Jobs(chart.Namespace).Get(context.TODO(), chart.Status.JobName, metav1.GetOptions{})
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

// GetJobCondition returns true if there is a condition on the job matching the selected type and status
func (f *Framework) GetJobCondition(job *batchv1.Job, condition batchv1.JobConditionType, status corev1.ConditionStatus) bool {
	for _, v := range job.Status.Conditions {
		if v.Type == condition && v.Status == status {
			return true
		}
	}
	return false
}
