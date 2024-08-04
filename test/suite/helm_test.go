package suite_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/k3s-io/helm-controller/test/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

var bootstrapTolerations = &[]corev1.Toleration{
	{
		Key:    "node.kubernetes.io/not-ready",
		Effect: "NoSchedule",
	},
	{
		Key:      "node.cloudprovider.kubernetes.io/uninitialized",
		Operator: "Equal",
		Value:    "true",
		Effect:   "NoSchedule",
	},
	{
		Key:      "CriticalAddonsOnly",
		Operator: "Exists",
	},
	{
		Key:      "node-role.kubernetes.io/etcd",
		Operator: "Exists",
		Effect:   "NoExecute",
	},
	{
		Key:      "node-role.kubernetes.io/control-plane",
		Operator: "Exists",
		Effect:   "NoSchedule",
	},
}

var _ = Describe("Helm Tests", Ordered, func() {
	framework, _ := framework.New()

	Context("When a helm V3 chart is created", func() {
		var (
			err     error
			chart   *v1.HelmChart
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a secret for the release", func() {
			Expect(secrets).To(HaveLen(1))
		})
	})

	Context("When a helm V3 chart is deleted", func() {
		var (
			chart   *v1.HelmChart
			secrets []corev1.Secret
			err     error
		)
		BeforeEach(func() {
			chart, err = framework.GetHelmChart("traefik-example", framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 0)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should remove the release from secrets and delete the chart", func() {
			Expect(secrets).To(HaveLen(0))

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a helm V3 chart version is updated", func() {
		var (
			err     error
			chart   *v1.HelmChart
			secrets []corev1.Secret
			pods    []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-update-example",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveLen(1))

			chart, err = framework.GetHelmChart(chart.Name, framework.Namespace)
			chart.Spec.Version = "1.86.2"
			chart, err = framework.UpdateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(chart.Spec.Version).To(Equal("1.86.2"))
			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should upgrade the release successfully", func() {
			Expect(pods[0].Status.ContainerStatuses[0].Image).To(BeEquivalentTo("docker.io/rancher/library-traefik:1.7.20"))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a helm V3 chart version is updated with values", func() {
		var (
			err     error
			chart   *v1.HelmChart
			secrets []corev1.Secret
			pods    []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-update-example-values",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveLen(1))

			chart, err = framework.GetHelmChart(chart.Name, framework.Namespace)
			chart.Spec.Set["replicas"] = intstr.FromString("3")
			chart, err = framework.UpdateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(chart.Spec.Set["replicas"]).To(Equal(intstr.FromString("3")))
			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 3)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should upgrade the release successfully", func() {
			Expect(len(pods)).To(BeEquivalentTo(3))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a helm V3 chart specifies a timeout", func() {
		var (
			err   error
			chart *v1.HelmChart
			pods  []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-timeout",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should install the release successfully", func() {
			Expect(len(pods)).To(BeEquivalentTo(1))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})

	})

	Context("When a helm V3 chart specifies ChartContent", func() {
		var (
			err   error
			chart *v1.HelmChart
			pods  []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-chartcontent",
				"",
				"1.86.1",
				"v3",
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

			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should install the release successfully", func() {
			Expect(len(pods)).To(BeEquivalentTo(1))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})

	})

	Context("When a helm V3 chart creates a namespace", func() {
		var (
			err     error
			chart   *v1.HelmChart
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-ns-example",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a secret and namespace for the release", func() {
			Expect(secrets).To(HaveLen(1))

			ns, err := framework.ClientSet.CoreV1().Namespaces().Get(context.Background(), chart.Spec.TargetNamespace, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ns).ToNot(BeNil())
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 0)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())

			err = framework.ClientSet.CoreV1().Namespaces().Delete(context.Background(), chart.Spec.TargetNamespace, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When a helm V2 chart is created", func() {
		var (
			err   error
			chart *v1.HelmChart
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-v2",
				"stable/traefik",
				"1.86.1",
				"v2",
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
		It("Chart should have failed condition", func() {
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
	})

	Context("When a custom backoffLimit is specified", func() {
		var (
			err          error
			chart        *v1.HelmChart
			job          *batchv1.Job
			backOffLimit int32
		)
		BeforeEach(func() {
			backOffLimit = 10
			chart = framework.NewHelmChart("traefik-example-custom-backoff",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct job backOff Limit", func() {
			Expect(*job.Spec.BackoffLimit).To(Equal(backOffLimit))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
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
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-default-backoff",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct job backOff Limit", func() {
			Expect(*job.Spec.BackoffLimit).To(Equal(defaultBackOffLimit))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a custom podSecurityContext is specified", func() {
		var (
			err                        error
			chart                      *v1.HelmChart
			job                        *batchv1.Job
			expectedPodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: pointer.BoolPtr(false),
			}
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-custom-podsecuritycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
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
				RunAsNonRoot: pointer.BoolPtr(false),
			}
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct pod securityContext", func() {
			Expect(*job.Spec.Template.Spec.SecurityContext).To(Equal(*expectedPodSecurityContext))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a no podSecurityContext is specified", func() {
		var (
			err                       error
			chart                     *v1.HelmChart
			job                       *batchv1.Job
			defaultPodSecurityContext = &corev1.PodSecurityContext{
				RunAsNonRoot: pointer.BoolPtr(true),
				SeccompProfile: &corev1.SeccompProfile{
					Type: "RuntimeDefault",
				},
			}
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-default-podsecuritycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct pod securityContext", func() {
			Expect(*job.Spec.Template.Spec.SecurityContext).To(Equal(*defaultPodSecurityContext))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a custom securityContext is specified", func() {
		var (
			err                     error
			chart                   *v1.HelmChart
			job                     *batchv1.Job
			expectedSecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: pointer.BoolPtr(true),
			}
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-custom-securitycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
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
				AllowPrivilegeEscalation: pointer.BoolPtr(true),
			}
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct container securityContext", func() {
			Expect(*job.Spec.Template.Spec.Containers[0].SecurityContext).To(Equal(*expectedSecurityContext))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When a no securityContext is specified", func() {
		var (
			err                    error
			chart                  *v1.HelmChart
			job                    *batchv1.Job
			defaultSecurityContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: pointer.BoolPtr(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{
						"ALL",
					},
				},
				ReadOnlyRootFilesystem: pointer.BoolPtr(true),
			}
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-default-securitycontext",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have correct container securityContext", func() {
			Expect(*job.Spec.Template.Spec.Containers[0].SecurityContext).To(Equal(*defaultSecurityContext))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When no tolerations are specified (non-boostrap)", func() {
		var (
			err   error
			chart *v1.HelmChart
			job   *batchv1.Job
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-no-tolerations",
				"stable/traefik",
				"1.86.1",
				"v3",
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

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have no tolerations set", func() {
			Expect(job.Spec.Template.Spec.Tolerations).To(BeEmpty())
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When custom tolerations are specified (non-boostrap)", func() {
		var (
			err         error
			chart       *v1.HelmChart
			job         *batchv1.Job
			tolerations = &[]corev1.Toleration{
				{
					Key:    "test-1",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "test-2",
					Effect: corev1.TaintEffectNoExecute,
				},
			}
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-custom-tolerations",
				"stable/traefik",
				"1.86.1",
				"v3",
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
			chart.Spec.Tolerations = *tolerations
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have tolerations set", func() {
			Expect(job.Spec.Template.Spec.Tolerations).To(Equal(*tolerations))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When no tolerations are specified (boostrap)", func() {
		var (
			err   error
			chart *v1.HelmChart
			job   *batchv1.Job
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-bootstrap-default-tolerations",
				"stable/traefik",
				"1.86.1",
				"v3",
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
			chart.Spec.Bootstrap = true
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have default tolerations set", func() {
			Expect(job.Spec.Template.Spec.Tolerations).To(Equal(*bootstrapTolerations))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

	Context("When custom tolerations are specified (boostrap)", func() {
		var (
			err             error
			chart           *v1.HelmChart
			job             *batchv1.Job
			specTolerations = &[]corev1.Toleration{
				{
					Key:    "test-1",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "test-2",
					Effect: corev1.TaintEffectNoExecute,
				},
			}
			finalTolerations = append(*bootstrapTolerations, *specTolerations...)
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-bootstrap-custom-tolerations",
				"stable/traefik",
				"1.86.1",
				"v3",
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
			chart.Spec.Tolerations = *specTolerations
			chart.Spec.Bootstrap = true
			chart, err = framework.CreateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			_, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())

			chart, err = framework.GetHelmChart(chart.Name, chart.Namespace)
			Expect(err).ToNot(HaveOccurred())
			job, err = framework.GetJob(chart)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should have tolerations set", func() {
			Expect(job.Spec.Template.Spec.Tolerations).To(Equal(finalTolerations))
		})
		AfterEach(func() {
			err = framework.DeleteHelmChart(chart.Name, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				_, err := framework.GetHelmChart(chart.Name, framework.Namespace)
				return err != nil && apierrors.IsNotFound(err)
			}, 120*time.Second, 5*time.Second).Should(BeTrue())
		})
	})

})
