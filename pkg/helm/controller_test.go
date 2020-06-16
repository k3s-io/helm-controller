package helm

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/rancher/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestInstallJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	job, _, _ := job(chart)
	assert.Equal("helm-install-traefik", job.Name)
	assert.Equal(image, job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal("helm-traefik", job.Spec.Template.Spec.ServiceAccountName)
}

func TestDeleteJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := v12.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	job, _, _ := job(chart)
	assert.Equal("helm-delete-traefik", job.Name)
}

func TestInstallArgs(t *testing.T) {
	assert := assert.New(t)
	stringArgs := strings.Join(args(NewChart()), " ")
	assert.Equal("install --set-string acme.dnsProvider.name=cloudflare --set rbac.enabled=true --set ssl.enabled=false", stringArgs)
}

func TestDeleteArgs(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := v12.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	stringArgs := strings.Join(args(chart), " ")
	assert.Equal("delete", stringArgs)
}

func NewChart() *v1.HelmChart {
	var set = make(map[string]intstr.IntOrString)
	set["rbac.enabled"] = intstr.IntOrString{StrVal: "true"}
	set["ssl.enabled"] = intstr.IntOrString{StrVal: "false"}
	set["acme.dnsProvider.name"] = intstr.IntOrString{StrVal: "cloudflare"}

	return v1.NewHelmChart("kube-system", "traefik", v1.HelmChart{
		Spec: v1.HelmChartSpec{
			Chart: "stable/traefik",
			Set:   set,
		},
	})
}
