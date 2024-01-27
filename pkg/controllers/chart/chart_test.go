package chart

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func TestHashObjects(t *testing.T) {
	type args struct {
		chartValuesContent  string
		configValuesContent string
		hash                string
	}

	tests := map[string]args{
		"No Values": {
			hash: "SHA256=E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
		},
		"Chart Only 1": {
			hash:               "SHA256=774949318A591C34D10D828B9D44F525DCBB34E2249BE2DB0C2FA52BC2A605FD",
			chartValuesContent: "foo: bar\n",
		},
		"Chart Only 2": {
			hash:               "SHA256=0A37EDD1B2E02066D78A9849F8575148F3832B753988AF32390C2A6C17D9E3F8",
			chartValuesContent: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
		},
		"Chart Only 3": {
			hash:               "SHA256=550D2692E1933725FF53C903624BC71D3C8F2053827462D1CF6F8AFFD7C29935",
			chartValuesContent: "{}",
		},
		"Config Only 1": {
			hash:                "SHA256=965E6A0F61A897F00296B3DB056CB5CEB2751501B55B8B16BAFDA3B97CA085B3",
			configValuesContent: "foo: baz\n",
		},
		"Config Only 2": {
			hash:                "SHA256=7D65D6E3BC1AF42C10A6EC8AACD6B7638C433DFB3B96A1D0DB2FBFAA5F4B7BBC",
			configValuesContent: "foo:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
		"Config Only 3": {
			hash:                "SHA256=2AE328666F8BA8A27D089C2E6CE3263FD98E827FB1371999D9C762EFB0D81E2B",
			configValuesContent: "{}",
		},
		"Chart and Config 1": {
			hash:                "SHA256=9F8063ED2A5BEA23BD634CDD649B4E0999E64977244246B8EEA1A916E568601F",
			chartValuesContent:  "foo: bar\n",
			configValuesContent: "foo: baz\n",
		},
		"Chart and Config 2": {
			hash:                "SHA256=78CCC0186D0E09A881708E668C9E47810DDF53F726361027328579FA22F4A3A0",
			chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			chart := NewChart()
			config := &v1.HelmChartConfig{}
			chart.Spec.ValuesContent = test.chartValuesContent
			config.Spec.ValuesContent = test.configValuesContent

			job, secret, configMap := job(chart, "6443")
			valuesSecretAddConfig(secret, config)

			assert.Nil(secret.StringData, "Secret StringData should be nil")
			assert.Nil(configMap.BinaryData, "ConfigMap BinaryData should be nil")

			if test.chartValuesContent == "" && test.configValuesContent == "" {
				assert.Empty(secret.Data, "Secret Data should be empty if HelmChart and HelmChartConfig ValuesContent are empty")
			} else {
				assert.NotEmpty(secret.Data, "Secret Data should not be empty if HelmChart and/or HelmChartConfig ValuesContent are not empty")
			}

			hashObjects(job, secret, configMap)
			assert.Equalf(test.hash, job.Spec.Template.ObjectMeta.Annotations[Annotation], "%s annotation value does not match", Annotation)
		})
	}
}

func TestSetVals(t *testing.T) {
	assert := assert.New(t)
	tests := map[string]bool{
		"":      false,
		" ":     false,
		"foo":   false,
		"1.0":   false,
		"0.1":   false,
		"0":     true,
		"1":     true,
		"-1":    true,
		"true":  true,
		"TrUe":  true,
		"false": true,
		"FaLsE": true,
		"null":  true,
		"NuLl":  true,
	}
	for testString, isTyped := range tests {
		ret := typedVal(intstr.Parse(testString))
		assert.Equal(isTyped, ret, "expected typedVal(%s) = %t", testString, isTyped)
	}
}

func TestInstallJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	job, _, _ := job(chart, "6443")
	assert.Equal("helm-install-traefik", job.Name)
	assert.Equal(DefaultJobImage, job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal("helm-traefik", job.Spec.Template.Spec.ServiceAccountName)
}

func TestDeleteJob(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := metav1.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	job, _, _ := job(chart, "6443")
	assert.Equal("helm-delete-traefik", job.Name)
}

func TestInstallJobImage(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	chart.Spec.JobImage = "custom-job-image"
	job, _, _ := job(chart, "6443")
	assert.Equal("custom-job-image", job.Spec.Template.Spec.Containers[0].Image)
}

func TestInstallArgs(t *testing.T) {
	assert := assert.New(t)
	stringArgs := strings.Join(args(NewChart()), " ")
	assert.Equal("install "+
		"--set-string acme.dnsProvider.name=cloudflare "+
		"--set-string global.clusterCIDR=10.42.0.0/16\\,fd42::/48 "+
		"--set-string global.systemDefaultRegistry= "+
		"--set rbac.enabled=true "+
		"--set ssl.enabled=false",
		stringArgs)
}

func TestDeleteArgs(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	deleteTime := metav1.NewTime(time.Time{})
	chart.DeletionTimestamp = &deleteTime
	stringArgs := strings.Join(args(chart), " ")
	assert.Equal("delete", stringArgs)
}

func NewChart() *v1.HelmChart {
	return v1.NewHelmChart("kube-system", "traefik", v1.HelmChart{
		Spec: v1.HelmChartSpec{
			Chart: "stable/traefik",
			Set: map[string]intstr.IntOrString{
				"rbac.enabled":                 intstr.Parse("true"),
				"ssl.enabled":                  intstr.Parse("false"),
				"acme.dnsProvider.name":        intstr.Parse("cloudflare"),
				"global.clusterCIDR":           intstr.Parse("10.42.0.0/16,fd42::/48"),
				"global.systemDefaultRegistry": intstr.Parse(""),
			},
		},
	})
}
