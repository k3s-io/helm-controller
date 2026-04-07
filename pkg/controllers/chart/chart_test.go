package chart

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/extjson"

	"github.com/rancher/wrangler/v3/pkg/yaml"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func TestHashObjects(t *testing.T) {
	type args struct {
		chartValues         string
		chartValuesContent  string
		configValues        string
		configValuesContent string
		hash                string
	}

	tests := map[string]args{
		"No Values": {
			hash: "SHA256=E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
		},
		"Chart Only 1": {
			hash:               "SHA256=B7D684A932E5B3AC74E009951700E032CE9936BF6BE82CD2DED22B5EA647EE5D",
			chartValuesContent: "foo: bar\n",
		},
		"Chart Only 2": {
			hash:               "SHA256=F3756AFACE793965D81AE9E9BD85A51369E60C18FE024E4D950BF56054258070",
			chartValuesContent: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
		},
		"Chart Only 3": {
			hash:               "SHA256=FFE4DB5EFB61ACC03F197C464414B5BB65885E8F03AE11B9EBB657D5DD3CCC55",
			chartValuesContent: "{}",
		},
		"Chart Only 4": {
			hash:        "SHA256=EA4FB70C0432FC1EEC700C96FA28530DD2B47A84D09F33AC2B9D666FA887C302",
			chartValues: "foo: bar\n",
		},
		"Config Only 1": {
			hash:                "SHA256=E00641CFFEB2D8EA3403D56DD456DAAF9578B4871F2FDB41B0F1AA33C25B69AF",
			configValuesContent: "foo: baz\n",
		},
		"Config Only 2": {
			hash:                "SHA256=309A32E491B3F0F43432948D90B4E766A278D0A3B3220E691EE35BC6429ECB52",
			configValuesContent: "foo:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
		"Config Only 3": {
			hash:                "SHA256=E1D81D53C173950A8F35BB397759CF49B3F43C0C797AD4F7C7AD6A3A47180E03",
			configValuesContent: "{}",
		},
		"Config Only 4": {
			hash:                "SHA256=88F5E5BF9826DD95940FC3DC702C5E69F46BA280D6C6E688875DFCD56FB8F629",
			configValues:        "foo: bar\n",
			configValuesContent: "foo: baz\n",
		},
		"Chart and Config 1": {
			hash:                "SHA256=F81EFF0BAF43F57D87FB53BCFAB06271091B411C4A582FCC130C33951CB7C81D",
			chartValuesContent:  "foo: bar\n",
			configValuesContent: "foo: baz\n",
		},
		"Chart and Config 2": {
			hash:                "SHA256=E41407A16AAC1DBD0B6D00A1818B0A73B0EB9A506131F3CAFD102ED751A8AA3D",
			chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
		"Chart and Config 3": {
			hash:         "SHA256=2C7889180BF017CF2F09368453178255F7E10B4883134AFE660CFF61D55CE20D",
			chartValues:  "foo: bar\n",
			configValues: "foo: baz\n",
		},
		"Chart and Config 4": {
			hash:                "SHA256=D4FA2B666B5A61C632A5AFD92BBF776279DB51D24357C4A461D1088135562DE4",
			chartValues:         "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			configValues:        "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
			configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)
			chart := NewChart()
			config := &v1.HelmChartConfig{}
			chart.Spec.Values = extjson.TryFromYAML(test.chartValues)
			chart.Spec.ValuesContent = test.chartValuesContent
			config.Spec.Values = extjson.TryFromYAML(test.configValues)
			config.Spec.ValuesContent = test.configValuesContent

			job, secret, configMap := job(chart, "6443")
			objects := []metav1.Object{configMap, secret}

			valuesSecretAddConfig(job, secret, config)

			assert.Nil(secret.StringData, "Secret StringData should be nil")
			assert.Nil(configMap.BinaryData, "ConfigMap BinaryData should be nil")

			if test.chartValues == "" && test.chartValuesContent == "" && test.configValues == "" && test.configValuesContent == "" {
				assert.Empty(secret.Data, "Secret Data should be empty if HelmChart and HelmChartConfig Values and ValuesContent are empty")
			} else {
				assert.NotEmpty(secret.Data, "Secret Data should not be empty if HelmChart and/or HelmChartConfig ValuesContent are not empty")
			}

			hashObjects(job, objects...)

			b, _ := yaml.ToBytes([]runtime.Object{job})
			t.Logf("Generated Job:\n%s", b)
			s, _ := yaml.ToBytes([]runtime.Object{secret})
			t.Logf("Generated Secret:\n%s", s)

			assert.Equalf(test.hash, job.Spec.Template.ObjectMeta.Annotations[AnnotationConfigHash], "%s annotation value does not match", AnnotationConfigHash)
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
	assert.Equal("32", job.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String())
	assert.Equal("32G", job.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String())
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

func TestDriverField(t *testing.T) {
	tests := []struct {
		name     string
		driver   v1.HelmDriver
		expected string
	}{
		{"default driver", "", "secret"},
		{"secret driver", "secret", "secret"},
		{"configmap driver", "configmap", "configmap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			chart := NewChart()
			chart.Spec.Driver = tt.driver
			j, _, _ := job(chart, "6443")
			envs := j.Spec.Template.Spec.Containers[0].Env
			var helmDriver string
			for _, e := range envs {
				if e.Name == "HELM_DRIVER" {
					helmDriver = e.Value
					break
				}
			}
			assert.Equal(tt.expected, helmDriver)
		})
	}
}

func TestMaxReleaseRevision(t *testing.T) {
	tests := []struct {
		name     string
		objects  []metav1.ObjectMeta
		expected int64
	}{
		{"no objects", nil, 0},
		{"single revision", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "1"}},
		}, 1},
		{"multiple revisions returns max", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "1"}},
			{Labels: map[string]string{"version": "3"}},
			{Labels: map[string]string{"version": "2"}},
		}, 3},
		{"invalid version label ignored", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "abc"}},
			{Labels: map[string]string{"version": "2"}},
		}, 2},
		{"missing version label ignored", []metav1.ObjectMeta{
			{Labels: map[string]string{"owner": "helm"}},
			{Labels: map[string]string{"version": "5"}},
		}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			assert.Equal(tt.expected, maxReleaseRevision(tt.objects))
		})
	}
}

func TestGetChartReleaseRevision(t *testing.T) {
	t.Run("configmap driver uses configmap storage", func(t *testing.T) {
		assert := assert.New(t)
		var called bool
		c := &Controller{
			configMaps: fakeConfigMapLister{
				list: func(namespace string, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
					called = true
					assert.Equal("target-ns", namespace)
					assert.Equal(labels.Set{"owner": "helm", "name": "traefik"}.AsSelector().String(), opts.LabelSelector)
					assert.Empty(opts.FieldSelector)
					return &corev1.ConfigMapList{
						Items: []corev1.ConfigMap{
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "1"}}},
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "3"}}},
						},
					}, nil
				},
			},
		}

		chart := NewChart()
		chart.Spec.Driver = "configmap"
		chart.Spec.TargetNamespace = "target-ns"

		revision, err := c.getChartReleaseRevision(chart)
		assert.NoError(err)
		assert.True(called)
		assert.Equal(int64(3), revision)
	})

	t.Run("default driver uses secret storage", func(t *testing.T) {
		assert := assert.New(t)
		var called bool
		c := &Controller{
			secrets: fakeSecretLister{
				list: func(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error) {
					called = true
					assert.Equal("target-ns", namespace)
					assert.Equal(labels.Set{"owner": "helm", "name": "traefik"}.AsSelector().String(), opts.LabelSelector)
					assert.Equal(fields.OneTermEqualSelector("type", ReleaseType).String(), opts.FieldSelector)
					return &corev1.SecretList{
						Items: []corev1.Secret{
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "2"}}},
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "5"}}},
						},
					}, nil
				},
			},
		}

		chart := NewChart()
		chart.Spec.TargetNamespace = "target-ns"

		revision, err := c.getChartReleaseRevision(chart)
		assert.NoError(err)
		assert.True(called)
		assert.Equal(int64(5), revision)
	})
}

type fakeConfigMapLister struct {
	list func(namespace string, opts metav1.ListOptions) (*corev1.ConfigMapList, error)
}

func (f fakeConfigMapLister) List(namespace string, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
	return f.list(namespace, opts)
}

type fakeSecretLister struct {
	list func(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error)
}

func (f fakeSecretLister) List(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error) {
	return f.list(namespace, opts)
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
