package crd

import (
	"context"
	"io"
	"os"
	"path/filepath"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/yaml"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func WriteFile(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return Print(f)
}

func Print(out io.Writer) error {
	obj, err := Objects(false)
	if err != nil {
		return err
	}
	data, err := yaml.Export(obj...)
	if err != nil {
		return err
	}

	objV1Beta1, err := Objects(true)
	if err != nil {
		return err
	}
	dataV1Beta1, err := yaml.Export(objV1Beta1...)
	if err != nil {
		return err
	}

	data = append([]byte("{{- if .Capabilities.APIVersions.Has \"apiextensions.k8s.io/v1\" -}}\n"), data...)
	data = append(data, []byte("{{- else -}}\n---\n")...)
	data = append(data, dataV1Beta1...)
	data = append(data, []byte("{{- end -}}")...)
	_, err = out.Write(data)
	return err
}

func Objects(v1beta1 bool) (result []runtime.Object, err error) {
	for _, crdDef := range List() {
		if v1beta1 {
			crd, err := crdDef.ToCustomResourceDefinitionV1Beta1()
			if err != nil {
				return nil, err
			}
			result = append(result, crd)
		} else {
			crd, err := crdDef.ToCustomResourceDefinition()
			if err != nil {
				return nil, err
			}
			result = append(result, crd)
		}
	}
	return
}

func List() []crd.CRD {
	return append(
		[]crd.CRD{
			newCRD(&v1.HelmChart{}, func(c crd.CRD) crd.CRD {
				return c.WithCustomColumn(
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Job",
						Type:        "string",
						Description: "Job associated with updates to this chart",
						JSONPath:    ".status.jobName",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Chart",
						Type:        "string",
						Description: "Helm Chart name",
						JSONPath:    ".spec.chart",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Target Namespace",
						Type:        "string",
						Description: "Helm Chart target namespace",
						JSONPath:    ".spec.targetNamespace",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Version",
						Type:        "string",
						Description: "Helm Chart version",
						JSONPath:    ".spec.version",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Repo",
						Type:        "string",
						Description: "Helm Chart repository URL",
						JSONPath:    ".spec.repo",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Helm Version",
						Type:        "string",
						Description: "Helm version used to manage the selected chart",
						JSONPath:    ".spec.helmVersion",
					},
					apiextv1.CustomResourceColumnDefinition{
						Name:        "Bootstrap",
						Type:        "boolean",
						Description: "True if this is chart is needed to bootstrap the cluste",
						JSONPath:    ".spec.bootstrap",
					},
				)
			}),
			newCRD(&v1.HelmChartConfig{}, nil),
		})
}

func Create(ctx context.Context, cfg *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(cfg)
	if err != nil {
		return err
	}

	return factory.BatchCreateCRDs(ctx, List()...).BatchWait()
}

func newCRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "helm.cattle.io",
			Version: "v1",
		},
		NonNamespace: false,
		Status:       true,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}
