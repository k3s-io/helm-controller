package helm

import (
	"github.com/rancher/helm-controller/pkg/apis/helm.cattle.io/v1"
	jobsv1 "github.com/rancher/helm-controller/pkg/generated/controllers/batch/v1"
	jobsMock "github.com/rancher/helm-controller/pkg/generated/controllers/batch/v1/fakes"
	helmMock "github.com/rancher/helm-controller/pkg/generated/controllers/helm.cattle.io/v1/fakes"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/apply/injectors"
	"github.com/rancher/wrangler/pkg/objectset"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
	"time"
	"strings"
)

func TestHelmControllerOnChange(t *testing.T) {
	assert := assert.New(t)
	controller := NewMockHelmController()
	chart := NewChart()
	key := chart.Namespace + "/" + chart.Name
	helmChart, _ := controller.OnHelmChanged(key, NewChart())
	assert.Equal("helm-install-traefik", helmChart.Status.JobName)
}

func TestHelmControllerOnRemove(t *testing.T) {
	assert := assert.New(t)
	controller := NewMockHelmController()
	chart := NewChart()
	key := chart.Namespace + "/" + chart.Name
	deleteTime := v12.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	helmChart, _ := controller.OnHelmRemove(key, chart)
	assert.Equal("traefik", helmChart.Name)
	assert.Equal( "kube-system", helmChart.Namespace)
}

func TestInstallJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	job, _ := job(chart)
	assert.Equal("helm-install-traefik", job.Name)
	assert.Equal(image, job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal("helm-traefik", job.Spec.Template.Spec.ServiceAccountName)
}

func TestDeleteJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := v12.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	job, _ := job(chart)
	assert.Equal("helm-delete-traefik", job.Name)
}

func TestInstallArgs(t *testing.T) {
	assert := assert.New(t)
	stringArgs := strings.Join(args(NewChart()), " ")
	assert.Equal("install --name traefik stable/traefik --set-string rbac.enabled=true --set-string ssl.enabled=true", stringArgs)
}

func TestDeleteArgs(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := v12.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	stringArgs := strings.Join(args(chart), " ")
	assert.Equal("delete --purge traefik", stringArgs)
}

func NewChart() *v1.HelmChart {
	var set = make(map[string]intstr.IntOrString)
	set["rbac.enabled"] = intstr.IntOrString{StrVal: "true"}
	set["ssl.enabled"] = intstr.IntOrString{StrVal: "true"}

	return v1.NewHelmChart("kube-system", "traefik", v1.HelmChart{
		Spec: v1.HelmChartSpec{
			Chart: "stable/traefik",
			Set: set,
		},
	})
}

func NewMockHelmController() Controller {
	helms := &helmMock.HelmChartControllerMock{
		UpdateFunc: func(in1 *v1.HelmChart) (*v1.HelmChart, error) {
			return in1, nil
		},
	}

	jobs := &jobsMock.JobControllerMock{
		CacheFunc: func () jobsv1.JobCache {
			return &jobsMock.JobCacheMock{
				GetFunc: func(namespace string, name string) (*batchv1.Job, error) {
					return &batchv1.Job{
						Status: batchv1.JobStatus{
							Succeeded: 0,
						},
					}, nil
				},
			}
		},
	}

	return Controller{
		helmController: helms,
		jobsCache:  jobs.Cache(),
		apply: &ApplyMock{},
	}
}

type ApplyMock struct {}
func (a ApplyMock) Apply(set *objectset.ObjectSet) error {
	return nil
}

func (a ApplyMock) WithCacheTypes(igs ...apply.InformerGetter) apply.Apply {
	return a
}
func (a ApplyMock) WithSetID(id string) apply.Apply {
	return a
}
func (a ApplyMock) WithOwner(obj runtime.Object) apply.Apply {
	return a
}
func (a ApplyMock) WithInjector(injs ...injectors.ConfigInjector) apply.Apply {
	return a
}
func (a ApplyMock) WithInjectorName(injs ...string) apply.Apply {
	return a
}
func (a ApplyMock) WithPatcher(gvk schema.GroupVersionKind, patchers apply.Patcher) apply.Apply {
	return a
}
func (a ApplyMock) WithStrictCaching() apply.Apply {
	return a
}
func (a ApplyMock) WithDefaultNamespace(ns string) apply.Apply {
	return a
}