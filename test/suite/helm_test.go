package suite_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	helmapiv1 "github.com/rancher/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/helm-controller/test/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Helm Tests", func() {
	framework, _ := framework.New()

	Context("When a helm V3 chart is created", func() {
		var (
			err     error
			chart   *helmapiv1.HelmChart
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example",
				"stable/traefik",
				"",
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
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a secret for the release", func() {
			Expect(secrets).To(HaveLen(1))
		})
	})

	Context("When a helm V3 chart is deleted", func() {
		var (
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart, err := framework.GetHelmChart("traefik-example", framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			err = framework.DeleteHelmChart("traefik-example", framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			labelSelector := labels.SelectorFromSet(labels.Set{
				"owner": "helm",
				"name":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 0)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should remove the release from secrets", func() {
			Expect(secrets).To(HaveLen(0))
		})
	})

	Context("When a helm V3 chart version is updated", func() {
		var (
			err     error
			chart   *helmapiv1.HelmChart
			secrets []corev1.Secret
			pods    []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-update-example",
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
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveLen(1))

			chart, err = framework.GetHelmChart("traefik-update-example", framework.Namespace)
			chart.Spec.Version = "1.86.2"
			chart, err = framework.UpdateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should upgrade the release successfully", func() {
			Expect(pods[0].Status.ContainerStatuses[0].Image).To(BeEquivalentTo("traefik:1.7.20"))
		})
	})

	Context("When a helm V2 chart is created", func() {
		var (
			err     error
			chart   *helmapiv1.HelmChart
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-example-v2",
				"stable/traefik",
				"",
				"v2",
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

			//avoid checking for jobs because they are finish quickly
			labelSelector := labels.SelectorFromSet(labels.Set{
				"OWNER": "TILLER",
				"NAME":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should create a secret for the release", func() {
			Expect(secrets).To(HaveLen(1))
		})
	})

	Context("When a helm V2 chart is deleted", func() {
		var (
			secrets []corev1.Secret
		)
		BeforeEach(func() {
			chart, err := framework.GetHelmChart("traefik-example-v2", framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			err = framework.DeleteHelmChart("traefik-example-v2", framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			labelSelector := labels.SelectorFromSet(labels.Set{
				"OWNER": "TILLER",
				"NAME":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 0)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should remove the release from secrets", func() {
			Expect(secrets).To(HaveLen(0))
		})
	})

	Context("When a helm V2 chart version is updated", func() {
		var (
			err     error
			chart   *helmapiv1.HelmChart
			secrets []corev1.Secret
			pods    []corev1.Pod
		)
		BeforeEach(func() {
			chart = framework.NewHelmChart("traefik-update-example-v2",
				"stable/traefik",
				"1.86.1",
				"v2",
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
				"OWNER": "TILLER",
				"NAME":  chart.Name,
			})
			secrets, err = framework.WaitForRelease(chart, labelSelector, 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(secrets).To(HaveLen(1))

			chart, err = framework.GetHelmChart("traefik-update-example-v2", framework.Namespace)
			chart.Spec.Version = "1.86.2"
			chart, err = framework.UpdateHelmChart(chart, framework.Namespace)
			Expect(err).ToNot(HaveOccurred())
			pods, err = framework.WaitForChartApp(chart, "traefik", 120*time.Second, 1)
			Expect(err).ToNot(HaveOccurred())
		})
		It("Should upgrade the release successfully", func() {
			Expect(pods[0].Status.ContainerStatuses[0].Image).To(BeEquivalentTo("traefik:1.7.20"))
		})
	})
})
