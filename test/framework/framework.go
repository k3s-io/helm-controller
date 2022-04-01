package framework

import (
	"context"
	"os"
	"time"

	"k8s.io/client-go/util/retry"

	helmapiv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	helm "github.com/k3s-io/helm-controller/pkg/controllers/chart"
	helmcln "github.com/k3s-io/helm-controller/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
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
	ginkgo.BeforeSuite(framework.BeforeSuite)
	ginkgo.AfterSuite(framework.AfterSuite)
	return framework, nil
}

func (f *Framework) BeforeSuite() {
	f.beforeFramework()
	err := f.setupController(context.TODO())
	if err != nil {
		errExit("Failed to set up helm controller", err)
	}
}

func (f *Framework) AfterSuite() {
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
	f.crds, err = getCRDs()
	errExit("Failed to construct helm crd", err)

	f.HelmClientSet = helmcln
	f.ClientSet = clientset
	f.crdFactory = crdFactory
	f.Name = helm.Name
	f.Namespace = helm.Name

}

func errExit(msg string, err error) {
	if err == nil {
		return
	}
	logrus.Panicf("%s: %v", msg, err)
}

func getCRDs() ([]crd.CRD, error) {
	var crds []crd.CRD
	for _, crdFn := range []func() (*crd.CRD, error){
		ChartCRD,
		ConfigCRD,
	} {
		crdef, err := crdFn()
		if err != nil {
			return nil, err
		}
		crds = append(crds, *crdef)
	}

	return crds, nil
}

func ChartCRD() (*crd.CRD, error) {
	prototype := helmapiv1.NewHelmChart("", "", helmapiv1.HelmChart{})
	schema, err := openapi.ToOpenAPIFromStruct(*prototype)
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        prototype.GroupVersionKind(),
		PluralName: helmapiv1.HelmChartResourceName,
		Status:     true,
		Schema:     schema,
	}, nil
}

func ConfigCRD() (*crd.CRD, error) {
	prototype := helmapiv1.NewHelmChartConfig("", "", helmapiv1.HelmChartConfig{})
	schema, err := openapi.ToOpenAPIFromStruct(*prototype)
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        prototype.GroupVersionKind(),
		PluralName: helmapiv1.HelmChartConfigResourceName,
		Status:     true,
		Schema:     schema,
	}, nil
}

func (f *Framework) NewHelmChart(name, chart, version, helmVersion string, set map[string]intstr.IntOrString) *helmapiv1.HelmChart {
	return &helmapiv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.Namespace,
			Labels: map[string]string{
				"helm-test": "true",
			},
		},
		Spec: helmapiv1.HelmChartSpec{
			Chart:       chart,
			Version:     version,
			Repo:        "",
			Set:         set,
			HelmVersion: helmVersion,
		},
	}
}

func (f *Framework) WaitForRelease(chart *helmapiv1.HelmChart, labelSelector labels.Selector, timeout time.Duration, count int) (secrets []corev1.Secret, err error) {

	return secrets, wait.Poll(5*time.Second, timeout, func() (bool, error) {
		list, err := f.ClientSet.CoreV1().Secrets(chart.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			return false, err
		}
		secrets = list.Items
		return len(secrets) == count, nil
	})
}

func (f *Framework) CreateHelmChart(chart *helmapiv1.HelmChart, namespace string) (*helmapiv1.HelmChart, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Create(context.TODO(), chart, metav1.CreateOptions{})
}

func (f *Framework) UpdateHelmChart(chart *helmapiv1.HelmChart, namespace string) (updated *helmapiv1.HelmChart, err error) {
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

func (f *Framework) GetHelmChart(name, namespace string) (*helmapiv1.HelmChart, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (f *Framework) ListHelmCharts(labelSelector, namespace string) (*helmapiv1.HelmChartList, error) {
	return f.HelmClientSet.HelmV1().HelmCharts(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// WaitForChartApp will check the for pods created by the chart
func (f *Framework) WaitForChartApp(chart *helmapiv1.HelmChart, appName string, timeout time.Duration, count int) (pods []corev1.Pod, err error) {
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
