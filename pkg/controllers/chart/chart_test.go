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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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
		deleted             bool
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "No Values",
			args: args{
				hash: "SHA256=7B9FDCEF22985143DB8EBC3123CCF6949B9F7767C3331DF397DD9E3A50F527D3",
			},
		},
		{
			name: "Chart Only 1",
			args: args{
				hash:               "SHA256=67C418FF0E52EE28676386CB75915B66C0F07CB541F2DE2010B41660635B4A8D",
				chartValuesContent: "foo: bar\n",
			},
		},
		{
			name: "Chart Only 2",
			args: args{
				hash:               "SHA256=61629635D20D65D6F5CEC25BA06793937565E7CDF3DF96202F585DEFE5D50306",
				chartValuesContent: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			},
		},
		{
			name: "Chart Only 3",
			args: args{
				hash:               "SHA256=EA59591A5463E5E22DA5C22DDA1A168C65EEB56194B94119CA4C399B88F9F3C5",
				chartValuesContent: "{}",
			},
		}, {
			name: "Chart Only 4",
			args: args{
				hash:        "SHA256=957717A3D8724FF4EFF73A90A134537201EFD9FBB920C2516E1045B0B754A711",
				chartValues: "foo: bar\n",
			},
		}, {
			name: "Config Only 1",
			args: args{
				hash:                "SHA256=15B3ABD9846881929F40C8EC24DE8EC4408BD4D3F0FB419E917AA66FD7E16911",
				configValuesContent: "foo: baz\n",
			},
		},
		{
			name: "Config Only 2",
			args: args{
				hash:                "SHA256=9D36C84621E5E36D8EFA427EEE5D88FC4FC09B58990AE17644ABEF6517ECB5E8",
				configValuesContent: "foo:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {

			name: "Config Only 3",
			args: args{
				hash:                "SHA256=D51FDD6AEEFAA1A1EE54D2636BF7A46A2128764911902F33AC2BF6DBE3F1CD8D",
				configValuesContent: "{}",
			},
		}, {
			name: "Config Only 4",
			args: args{
				hash:                "SHA256=0F574C7C5756D0EFF5B89B550824C5ED99DF6AF809629DDD9204E4BBDFC397FD",
				configValues:        "foo: bar\n",
				configValuesContent: "foo: baz\n",
			},
		}, {
			name: "Chart and Config 1",
			args: args{
				hash:                "SHA256=B86FFB50BF565CA143439489CF8F503B6AF098E17A0C3D0F69080E8D41F8B4CC",
				chartValuesContent:  "foo: bar\n",
				configValuesContent: "foo: baz\n",
			},
		}, {
			name: "Chart and Config 2",
			args: args{
				hash:                "SHA256=D0F1C546974B380D11B3A33F893AF0AE7C3131ACFE7FE22476C2AAA7E6160A43",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {
			name: "Chart and Config 3",
			args: args{
				hash:         "SHA256=586EEB058FB147690F546AEAE7C238A551759C08881E3BFCE7544A7FAFAC8187",
				chartValues:  "foo: bar\n",
				configValues: "foo: baz\n",
			},
		}, {
			name: "Chart and Config 4",
			args: args{
				hash:                "SHA256=FE2783C8C7924587AC654A29AF97911503A6C704F3DECD3C4DA80B24703CECC8",
				chartValues:         "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValues:        "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {
			// note: both deleted charts have the same hash, as values secrets and content configmaps are not generated when deleting
			name: "Deleted 1",
			args: args{
				hash:                "SHA256=0807D189F31BF3EB82FA02EFB047A110F132004D37C50B02C8238AD07CC281D1",
				chartValues:         "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValues:        "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				deleted:             true,
			},
		}, {
			name: "Deleted 2",
			args: args{
				hash:        "SHA256=0807D189F31BF3EB82FA02EFB047A110F132004D37C50B02C8238AD07CC281D1",
				chartValues: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				deleted:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			chart := NewChart()
			config := &v1.HelmChartConfig{}
			test := tt.args
			chart.Spec.Values = extjson.TryFromYAML(test.chartValues)
			chart.Spec.ValuesContent = test.chartValuesContent
			config.Spec.Values = extjson.TryFromYAML(test.configValues)
			config.Spec.ValuesContent = test.configValuesContent
			if test.deleted {
				chart.DeletionTimestamp = ptr.To(metav1.Now())
			}

			job, secret, configMap := job(chart, "6443")

			objects := []metav1.Object{configMap, secret}
			if chart.DeletionTimestamp == nil {
				valuesSecretAddConfig(job, secret, config)

				assert.Nil(secret.StringData, "Secret StringData should be nil")
				assert.Nil(configMap.BinaryData, "ConfigMap BinaryData should be nil")

				if test.chartValues == "" && test.chartValuesContent == "" && test.configValues == "" && test.configValuesContent == "" {
					assert.Empty(secret.Data, "Secret Data should be empty if HelmChart and HelmChartConfig Values and ValuesContent are empty")
				} else {
					assert.NotEmpty(secret.Data, "Secret Data should not be empty if HelmChart and/or HelmChartConfig ValuesContent are not empty")
				}
			}

			hashObjects(job, objects...)

			b, _ := yaml.ToBytes([]runtime.Object{job})
			t.Logf("Generated Job:\n%s", b)
			s, _ := yaml.ToBytes([]runtime.Object{secret})
			t.Logf("Generated Secret:\n%s", s)

			assert.Equalf(test.hash, job.Spec.Template.ObjectMeta.Annotations[KeyConfigHash], "%s annotation value does not match", KeyConfigHash)
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
	oldJobResources := JobResources
	defer func() { JobResources = oldJobResources }()
	JobResources = &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10"),
			corev1.ResourceMemory: resource.MustParse("10G"),
		},
	}

	chart := NewChart()
	job, _, _ := job(chart, "6443")
	assert.Equal("helm-install-traefik", job.Name)
	assert.Equal(DefaultJobImage, job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal("helm-traefik", job.Spec.Template.Spec.ServiceAccountName)
	assert.Equal("10", job.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String())
	assert.Equal("10G", job.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String())
}

func TestInstallJobWithoutPodLimits(t *testing.T) {
	assert := assert.New(t)
	oldJobResources := JobResources
	defer func() { JobResources = oldJobResources }()
	JobResources = nil

	chart := NewChart()
	job, _, _ := job(chart, "6443")
	assert.Empty(job.Spec.Template.Spec.Containers[0].Resources.Requests)
	assert.Empty(job.Spec.Template.Spec.Containers[0].Resources.Limits)
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

func TestInstallJobTolerations(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	oldDefaultJobTolerations := JobTolerations
	defer func() { JobTolerations = oldDefaultJobTolerations }()
	JobTolerations = []corev1.Toleration{{
		Key:      "custom-taint",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}}

	job, _, _ := job(chart, "6443")
	assert.Contains(job.Spec.Template.Spec.Tolerations, JobTolerations[0])
}

func TestInstallJobBootstrapAndCustomTolerations(t *testing.T) {
	assert := assert.New(t)
	chart := NewChart()
	chart.Spec.Bootstrap = true
	oldDefaultJobTolerations := JobTolerations
	defer func() { JobTolerations = oldDefaultJobTolerations }()
	JobTolerations = []corev1.Toleration{{
		Key:      "custom-taint",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}}

	job, _, _ := job(chart, "6443")
	assert.GreaterOrEqual(len(job.Spec.Template.Spec.Tolerations), len(JobTolerations)+1)
	assert.Contains(job.Spec.Template.Spec.Tolerations, JobTolerations[0])
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
		expected release
	}{
		{"no objects", nil, release{}},
		{"single revision", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "1"}},
		}, release{revision: 1}},
		{"multiple revisions returns max", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "1"}},
			{Labels: map[string]string{"version": "3"}},
			{Labels: map[string]string{"version": "2"}},
		}, release{revision: 3}},
		{"invalid version label ignored", []metav1.ObjectMeta{
			{Labels: map[string]string{"version": "abc"}},
			{Labels: map[string]string{"version": "2"}},
		}, release{revision: 2}},
		{"missing version label ignored", []metav1.ObjectMeta{
			{Labels: map[string]string{"owner": "helm"}},
			{Labels: map[string]string{"version": "5"}},
		}, release{revision: 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			got, err := latestRelease(tt.objects)
			assert.NoError(err)
			assert.Equal(tt.expected, got)
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
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "3", "status": "deployed", KeyConfigHash: "ABC"}}},
						},
					}, nil
				},
			},
		}

		chart := NewChart()
		chart.Spec.Driver = "configmap"
		chart.Spec.TargetNamespace = "target-ns"

		rel, err := c.getChartRelease(chart)
		assert.NoError(err)
		assert.True(called)
		assert.Equal(release{revision: 3, status: "deployed", hash: "ABC"}, rel)
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
							{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version": "5", "status": "deployed", KeyConfigHash: "ABC"}}},
						},
					}, nil
				},
			},
		}

		chart := NewChart()
		chart.Spec.TargetNamespace = "target-ns"

		rel, err := c.getChartRelease(chart)
		assert.NoError(err)
		assert.True(called)
		assert.Equal(release{revision: 5, status: "deployed", hash: "ABC"}, rel)
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
