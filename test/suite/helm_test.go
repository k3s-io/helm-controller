package suite_test

import (
	"context"
	"fmt"
	"time"

	//revive:disable:dot-imports
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/test/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

var _ = Describe("HelmChart Controller Tests", Ordered, func() {
	framework, _ := framework.New()

	Context("When a HelmChart is created", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})

		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart version is updated", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-update-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should create a new release when the version is changed", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			chart.Spec.Version = "1.86.2"
			chart, err = framework.UpdateHelmChart(chart, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(chart.Spec.Version).To(Equal("1.86.2"))

			// check for 2 releases, and pod with image specified by new chart version
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))
			Eventually(framework.ListChartPods, 120*time.Second, 5*time.Second).WithArguments(chart, "traefik").Should(
				ContainElement(HaveField("Status.ContainerStatuses", ContainElements(HaveField("Image", ContainSubstring("docker.io/rancher/library-traefik:1.7.20"))))),
			)
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart version is changed", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-update-example-values",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should create a new release when the values are changed", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			chart.Spec.Set["replicas"] = intstr.FromString("3")
			chart, err = framework.UpdateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(chart.Spec.Set["replicas"]).To(Equal(intstr.FromString("3")))

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart is created with spec.takeOwnership=true", func() {
		var (
			err     error
			chart   *v1.HelmChart
			service *corev1.Service
		)
		BeforeAll(func() {
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traefik-example",
					Namespace: framework.Namespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "None",
					Type:      corev1.ServiceTypeClusterIP,
				},
			}
			service, err = framework.ClientSet.CoreV1().Services(framework.Namespace).Create(context.TODO(), service, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.TakeOwnership = true
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})

		It("Should take ownership of existing resources", func() {
			Eventually(func(g Gomega) {
				service, err = framework.ClientSet.CoreV1().Services(framework.Namespace).Get(context.TODO(), service.Name, metav1.GetOptions{})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(service).To(HaveField("ObjectMeta.Annotations", HaveKeyWithValue("meta.helm.sh/release-name", "traefik-example")))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart specifies a timeout", func() {
		var (
			err   error
			chart *v1.HelmChart
			job   *batchv1.Job
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-timeout",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.Timeout = &metav1.Duration{Duration: time.Minute * 15}

			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have the correct timeout", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Env).To(ContainElement(
				And(
					HaveField("Name", "TIMEOUT"),
					HaveField("Value", chart.Spec.Timeout.Duration.String()),
				),
			))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart specifies ChartContent", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-chartcontent",
				"",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.ChartContent, err = framework.GetChartContent("https://charts.helm.sh/stable/packages/traefik-1.86.1.tgz")
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart has HelmChartConfig", func() {
		var (
			err         error
			chart       *v1.HelmChart
			chartConfig *v1.HelmChartConfig
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"",
				nil)
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should create a new release when the HelmChartConfig is created", func() {
			chartConfig = framework.NewHelmChartConfig(chart.Name, "", "metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n")
			chartConfig, err = framework.CreateHelmChartConfig(chartConfig, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			err = framework.DeleteHelmChartConfig(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart has values", func() {
		var (
			err         error
			chart       *v1.HelmChart
			chartConfig *v1.HelmChartConfig
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"test:\n  1: chart.values\n  2: chart.values\n  4: chart.values",
				"test:\n  1: chart.valuesContent\n  2: chart.valuesContent\n  3: chart.valuesContent\n  4: chart.valuesContent\n  5: chart.valuesContent",
				map[string]intstr.IntOrString{
					"test.1": intstr.FromString("chart.set"),
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart with all values in the expected precedence (set > values > valuesContent)", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))

			config, err := framework.GetDeployedReleaseConfig(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).To(Equal(map[string]any{
				"test": map[string]any{
					"1": "chart.set",
					"2": "chart.values",
					"3": "chart.valuesContent",
					"4": "chart.values",
					"5": "chart.valuesContent",
				},
			}))
		})
		It("Should override/merge values from HelmChartConfig (chart.set > config.values > config.valuesContent > chart.values > chart.valuesContent)", func() {
			chartConfig = framework.NewHelmChartConfig(chart.Name,
				"test:\n  1: config.values\n  2: config.values",
				"test:\n  1: config.valuesContent\n  2: config.valuesContent\n  3: config.valuesContent",
			)
			chartConfig, err = framework.CreateHelmChartConfig(chartConfig, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))

			config, err := framework.GetDeployedReleaseConfig(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).To(Equal(map[string]any{
				"test": map[string]any{
					"1": "chart.set",
					"2": "config.values",
					"3": "config.valuesContent",
					"4": "chart.values",
					"5": "chart.valuesContent",
				},
			}))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			err = framework.DeleteHelmChartConfig(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart has ValuesSecrets", func() {
		var (
			err        error
			chart      *v1.HelmChart
			userSecret *corev1.Secret
		)
		BeforeAll(func() {
			userSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-traefik-values",
					Namespace: framework.Namespace,
				},
				StringData: map[string]string{
					"values.yaml": "metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				},
			}
			userSecret, err = framework.ClientSet.CoreV1().Secrets(userSecret.Namespace).Create(context.TODO(), userSecret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"",
				nil)

			chart.Spec.ValuesSecrets = []v1.SecretSpec{
				{
					Name: userSecret.Name,
					Keys: []string{"values.yaml"},
				},
			}

			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should create a new release when the secret is modified", func() {
			userSecret.Data = nil
			userSecret.StringData = map[string]string{
				"values.yaml": "metrics:\n  prometheus:\n    enabled: false\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
			}
			userSecret, err = framework.ClientSet.CoreV1().Secrets(userSecret.Namespace).Update(context.TODO(), userSecret, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))

			err = framework.ClientSet.CoreV1().Secrets(userSecret.Namespace).Delete(context.TODO(), userSecret.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When a HelmChart has HelmChartConfig ValuesSecrets", func() {
		var (
			err         error
			chart       *v1.HelmChart
			chartConfig *v1.HelmChartConfig
			userSecret  *corev1.Secret
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"",
				nil)
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should create a new release when the HelmChartConfig is created", func() {
			userSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-traefik-values",
					Namespace: framework.Namespace,
				},
				StringData: map[string]string{
					"values.yaml": "metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				},
			}

			userSecret, err = framework.ClientSet.CoreV1().Secrets(userSecret.Namespace).Create(context.TODO(), userSecret, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			chartConfig = framework.NewHelmChartConfig(chart.Name, "", "")
			chartConfig.Spec.ValuesSecrets = []v1.SecretSpec{
				{
					Name: userSecret.Name,
					Keys: []string{"values.yaml"},
				},
			}

			chartConfig, err = framework.CreateHelmChartConfig(chartConfig, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(2))
		})
		It("Should create a new release when the secret is modified", func() {
			userSecret.Data = nil
			userSecret.StringData = map[string]string{
				"values.yaml": "metrics:\n  prometheus:\n    enabled: false\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
			}
			userSecret, err = framework.ClientSet.CoreV1().Secrets(userSecret.Namespace).Update(context.TODO(), userSecret, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(3))
		})

		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			err = framework.DeleteHelmChartConfig(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart creates a namespace", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-ns-example",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.TargetNamespace = chart.Name
			chart.Spec.CreateNamespace = true
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart in the target namespace", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(
				And(
					HaveLen(1),
					ContainElement(HaveField("ObjectMeta.Namespace", Equal(chart.Spec.TargetNamespace))),
				))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart resided within the target namespace", Label("filter"), func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			name := "traefik-within-ns-example"
			err = framework.CreateNamespace(name, true)
			Expect(err).ToNot(HaveOccurred())
			chart = framework.NewHelmChart(name,
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should be possible to delete the namespace", func() {
			err = framework.DeleteNamespace(chart.Namespace, true)
			Expect(err).ToNot(HaveOccurred())
			Eventually(framework.ListNamespaces, 120*time.Second, 5*time.Second).WithArguments(chart.Namespace).Should(BeEmpty())
		})
		AfterAll(func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a HelmChart V2 is created", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-v2",
				"stable/traefik",
				"1.86.1",
				"v2",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have failed condition", func() {
			Eventually(func() error {
				chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
				if err != nil {
					return err
				}
				if !framework.GetHelmChartCondition(chart, v1.HelmChartFailed, corev1.ConditionTrue, "Unsupported version") {
					return fmt.Errorf("expected condition %v=%v not found", v1.HelmChartFailed, corev1.ConditionTrue)
				}
				return nil
			}, 120*time.Second).ShouldNot(HaveOccurred())
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a custom backoffLimit is specified", func() {
		var (
			err          error
			chart        *v1.HelmChart
			job          *batchv1.Job
			backOffLimit int32
		)
		BeforeAll(func() {
			backOffLimit = 10
			chart = framework.NewHelmChart("traefik-example-custom-backoff",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.BackOffLimit = &backOffLimit
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct job backOff Limit", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.BackoffLimit).To(Equal(backOffLimit))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a no backoffLimit is specified", func() {
		var (
			err   error
			chart *v1.HelmChart
			job   *batchv1.Job
		)
		const (
			defaultBackOffLimit = int32(1000)
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-default-backoff",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct job backOff Limit", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.BackoffLimit).To(Equal(defaultBackOffLimit))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a custom podSecurityContext is specified", func() {
		var (
			err                        error
			chart                      *v1.HelmChart
			job                        *batchv1.Job
			expectedPodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(false),
			}
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-custom-podsecuritycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.PodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(false),
			}
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct pod securityContext", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.Template.Spec.SecurityContext).To(Equal(*expectedPodSecurityContext))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a no podSecurityContext is specified", func() {
		var (
			err                       error
			chart                     *v1.HelmChart
			job                       *batchv1.Job
			defaultPodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				SeccompProfile: &corev1.SeccompProfile{
					Type: "RuntimeDefault",
				},
			}
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-default-podsecuritycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct pod securityContext", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)

			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.Template.Spec.SecurityContext).To(Equal(*defaultPodSecurityContext))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a custom securityContext is specified", func() {
		var (
			err                     error
			chart                   *v1.HelmChart
			job                     *batchv1.Job
			expectedSecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(true),
			}
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-custom-securitycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart.Spec.SecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(true),
			}
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct container securityContext", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.Template.Spec.Containers[0].SecurityContext).To(Equal(*expectedSecurityContext))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})

	Context("When a no securityContext is specified", func() {
		var (
			err                    error
			chart                  *v1.HelmChart
			job                    *batchv1.Job
			defaultSecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{
						"ALL",
					},
				},
				ReadOnlyRootFilesystem: ptr.To(true),
			}
		)
		BeforeAll(func() {
			chart = framework.NewHelmChart("traefik-example-default-securitycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
				"",
				"metrics:\n  prometheus:\n    enabled: true\nkubernetes:\n  ingressEndpoint:\n    useDefaultPublishedService: true\nimage: docker.io/rancher/library-traefik\n",
				map[string]intstr.IntOrString{
					"rbac.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
					"ssl.enabled": {
						Type:   intstr.String,
						StrVal: "true",
					},
				})
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a release for the chart", func() {
			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(1))
		})
		It("Should have correct container securityContext", func() {
			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
			Expect(*job.Spec.Template.Spec.Containers[0].SecurityContext).To(Equal(*defaultSecurityContext))
		})
		AfterAll(func() {
			err = framework.DeleteHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(framework.GetHelmChart(chart.Name, chart.Namespace)).Error().Should(MatchError(apierrors.IsNotFound, "IsNotFound"))
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			Eventually(framework.ListReleases, 120*time.Second, 5*time.Second).WithArguments(chart).Should(HaveLen(0))
		})
	})
})
