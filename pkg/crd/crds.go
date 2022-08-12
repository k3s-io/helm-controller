package crd

import (
	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/crd"
)

func List() []crd.CRD {
	chart := crd.NamespacedType("HelmChart.helm.cattle.io/v1").
		WithSchemaFromStruct(v1.HelmChart{}).
		WithColumn("Job", ".status.jobName").
		WithColumn("Chart", ".spec.chart").
		WithColumn("TargetNamespace", ".spec.targetNamespace").
		WithColumn("Version", ".spec.version").
		WithColumn("Repo", ".spec.repo").
		WithColumn("HelmVersion", ".spec.helmVersion").
		WithColumn("Bootstrap", ".spec.bootstrap")

	config := crd.NamespacedType("HelmChartConfig.helm.cattle.io/v1").
		WithSchemaFromStruct(v1.HelmChartConfig{})

	return []crd.CRD{chart, config}
}
