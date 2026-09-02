package chart

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/pkg/controllers/extjson"
	"go.uber.org/mock/gomock"

	wranglerfake "github.com/rancher/wrangler/v3/pkg/generic/fake"
	"github.com/rancher/wrangler/v3/pkg/yaml"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
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
		bootstrap           bool
		deleted             bool
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "No Values",
			args: args{
				hash: "SHA256=093BC73A1C288F832E33F887E0F1B23403188FCCAC57F1293FB19A7BA02D8A54",
			},
		}, {
			name: "No Values (bootstrap)",
			args: args{
				hash:      "SHA256=093BC73A1C288F832E33F887E0F1B23403188FCCAC57F1293FB19A7BA02D8A54",
				bootstrap: true,
			},
		}, {
			name: "Chart Only (bootstrap)",
			args: args{
				hash:               "SHA256=9647B243590942AC04A543760720DD200E99E5DF2C86DE15899338138FA9FE52",
				chartValuesContent: "foo: bar\n",
				bootstrap:          true,
			},
		}, {
			name: "Chart Only 1",
			args: args{
				hash:               "SHA256=9647B243590942AC04A543760720DD200E99E5DF2C86DE15899338138FA9FE52",
				chartValuesContent: "foo: bar\n",
			},
		}, {
			name: "Chart Only 2",
			args: args{
				hash:               "SHA256=2447EFBFF7EB3CDDD7D0D837D66DA0464C6EBC74F2FD3139969094ECB6AD83B3",
				chartValuesContent: "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
			},
		}, {
			name: "Chart Only 3",
			args: args{
				hash:               "SHA256=755203C2D22E6D546EF829CD0FC3B8DB67214406697A6B744275FE20229DE73C",
				chartValuesContent: "{}",
			},
		}, {
			name: "Chart Only 4",
			args: args{
				hash:        "SHA256=F45A4570241BE6597A2D91C17B5210B7F94C6EC8A4964E6010AB0E442AE9F582",
				chartValues: "foo: bar\n",
			},
		}, {
			name: "Config Only (bootstrap)",
			args: args{
				hash:                "SHA256=592698FA722B72294DE013D670CB6865A60FDA0BE8737CD8969B19B2AA738C51",
				configValuesContent: "foo: baz\n",
				bootstrap:           true,
			},
		}, {
			name: "Config Only 1",
			args: args{
				hash:                "SHA256=592698FA722B72294DE013D670CB6865A60FDA0BE8737CD8969B19B2AA738C51",
				configValuesContent: "foo: baz\n",
			},
		}, {
			name: "Config Only 2",
			args: args{
				hash:                "SHA256=6487F887DCDB89778080823D5181F8A2CA623112545B467E3E8E15388FADBFA0",
				configValuesContent: "foo:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {

			name: "Config Only 3",
			args: args{
				hash:                "SHA256=E2407E9D44172D6F29ACFEDDC835EC7FBF5DC0AFB21A7061123F9EC5865996E3",
				configValuesContent: "{}",
			},
		}, {
			name: "Config Only 4",
			args: args{
				hash:                "SHA256=0565CD4E9BC3E25AB21AF73DA4DB3C4DA9244DF5635CF5D4C272783244A0D514",
				configValues:        "foo: bar\n",
				configValuesContent: "foo: baz\n",
			},
		}, {
			name: "Chart and Config (bootstrap)",
			args: args{
				hash:                "SHA256=591076A4F560F437686B9F49876598B4AB61E6672BC3AD09046A46ADE5D59B6E",
				chartValuesContent:  "foo: bar\n",
				configValuesContent: "foo: baz\n",
				bootstrap:           true,
			},
		}, {
			name: "Chart and Config 1",
			args: args{
				hash:                "SHA256=591076A4F560F437686B9F49876598B4AB61E6672BC3AD09046A46ADE5D59B6E",
				chartValuesContent:  "foo: bar\n",
				configValuesContent: "foo: baz\n",
			},
		}, {
			name: "Chart and Config 2",
			args: args{
				hash:                "SHA256=2F969634ABEFAEBFA800C8A725EDC96DE5361F6AB0FBE3E4FCD48EF5B903D8AB",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {
			name: "Chart and Config 3",
			args: args{
				hash:         "SHA256=447A6CB9719F2A95476284DB44517E3F9A9395521427663DBBFD83A6CA4B2C7D",
				chartValues:  "foo: bar\n",
				configValues: "foo: baz\n",
			},
		}, {
			name: "Chart and Config 4",
			args: args{
				hash:                "SHA256=84143FB884E39F719D5041FE7C3486DD1C3DC08CDFCBA7CAAAE73F6F0F336FE2",
				chartValues:         "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValues:        "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
			},
		}, {
			// note: both deleted charts have the same hash, as values secrets and content configmaps are not generated when deleting
			name: "Deleted 1",
			args: args{
				hash:                "SHA256=F1D05B70BBD127B3488293FFE791D47E8D1DA79F96FEC32D7750892D7512598A",
				chartValues:         "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				chartValuesContent:  "foo:\n  a: true\n  b: 1\n  c: 'true'\n",
				configValues:        "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				configValuesContent: "bar:\n  a: false\n  b: 0\n  c: 'false'\n",
				deleted:             true,
			},
		}, {
			name: "Deleted 2",
			args: args{
				hash:        "SHA256=F1D05B70BBD127B3488293FFE791D47E8D1DA79F96FEC32D7750892D7512598A",
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
		ctrl := gomock.NewController(t)
		secrets := wranglerfake.NewMockControllerInterface[*corev1.Secret, *corev1.SecretList](ctrl)
		configmaps := wranglerfake.NewMockControllerInterface[*corev1.ConfigMap, *corev1.ConfigMapList](ctrl)

		client := fake.NewClientset(
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "target-ns", Name: "sh.helm.release.v1.traefik.v2", Labels: map[string]string{"name": "traefik", "owner": "helm", "version": "2"}}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "target-ns", Name: "sh.helm.release.v1.traefik.v3", Labels: map[string]string{"name": "traefik", "owner": "helm", "version": "3", "status": "deployed", KeyConfigHash: "ABC"}}},
		)
		configmaps.EXPECT().List(gomock.Any(), gomock.Any()).MinTimes(1).DoAndReturn(func(namespace string, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
			return client.CoreV1().ConfigMaps(namespace).List(t.Context(), opts)
		})

		c := &Controller{
			secrets:    secrets,
			configMaps: configmaps,
		}

		chart := NewChart()
		chart.Spec.Driver = "configmap"
		chart.Spec.TargetNamespace = "target-ns"

		rel, err := c.getChartRelease(chart)
		assert.NoError(err)
		assert.Equal(release{revision: 3, status: "deployed", hash: "ABC"}, rel)
	})

	t.Run("default driver uses secret storage", func(t *testing.T) {
		assert := assert.New(t)
		ctrl := gomock.NewController(t)
		secrets := wranglerfake.NewMockControllerInterface[*corev1.Secret, *corev1.SecretList](ctrl)
		configmaps := wranglerfake.NewMockControllerInterface[*corev1.ConfigMap, *corev1.ConfigMapList](ctrl)

		client := fake.NewClientset(
			&corev1.Secret{Type: "helm.sh/release.v1", ObjectMeta: metav1.ObjectMeta{Namespace: "target-ns", Name: "sh.helm.release.v1.traefik.v2", Labels: map[string]string{"name": "traefik", "owner": "helm", "version": "2"}}},
			&corev1.Secret{Type: "helm.sh/release.v1", ObjectMeta: metav1.ObjectMeta{Namespace: "target-ns", Name: "sh.helm.release.v1.traefik.v3", Labels: map[string]string{"name": "traefik", "owner": "helm", "version": "3", "status": "deployed", KeyConfigHash: "ABC"}}},
		)
		secrets.EXPECT().List(gomock.Any(), gomock.Any()).MinTimes(1).DoAndReturn(func(namespace string, opts metav1.ListOptions) (*corev1.SecretList, error) {
			return client.CoreV1().Secrets(namespace).List(t.Context(), opts)
		})

		c := &Controller{
			secrets:    secrets,
			configMaps: configmaps,
		}

		chart := NewChart()
		chart.Spec.TargetNamespace = "target-ns"

		rel, err := c.getChartRelease(chart)
		assert.NoError(err)
		assert.Equal(release{revision: 3, status: "deployed", hash: "ABC"}, rel)
	})
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
