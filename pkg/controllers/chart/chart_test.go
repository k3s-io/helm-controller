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
			hash:               "SHA256=F2430D3E9731D361C834918C8811603AB6C7B1ABF794714E6A049667FA26AF56",
			chartValuesContent: "foo: bar\n",
		},
		"Chart Only 2": {
			hash:               "SHA256=CFA019F5E68D9ECDC32113711AFE24D2A16BDCD6FDFE8C931AEB016B6571CA32",
			chartValuesContent: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
		},
		"Chart Only 3": {
			hash:               "SHA256=D29893D5711A98CE622ABB742144FD9B9CE4721EC74E5C2BE22514FDDC1B7B28",
			chartValuesContent: "{}",
		},
		"Config Only 1": {
			hash:                "SHA256=741B84D32D28AAEB48BB8AB0AC5E81C721FB7F1D388230FE512CAA98F27AE161",
			configValuesContent: "foo: baz\n",
		},
		"Config Only 2": {
			hash:                "SHA256=F1C538DA9A5416D68C9CE00CE087F5FBE262A2BF09AB4CA012A2CFB75EB7B3F2",
			configValuesContent: "foo:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
		"Config Only 3": {
			hash:                "SHA256=DF34CF6AA589A222578D73E6E8CE2D8FC8B725067CEAE8346DF5074788A3FF1F",
			configValuesContent: "{}",
		},
		"Chart and Config 1": {
			hash:                "SHA256=6E4947C029CC2A4950C271F7411BEE8B54CBD16D384703B67FF36BAE282FCC75",
			chartValuesContent:  "foo: bar\n",
			configValuesContent: "foo: baz\n",
		},
		"Chart and Config 2": {
			hash:                "SHA256=104D10F97C23BCC66EEA4B6F0489B6F3D6E414094C1DC300F2C123E68BD9BEF5",
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
		"--skip-crds "+
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
			Chart:    "stable/traefik",
			SkipCRDs: true,
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
