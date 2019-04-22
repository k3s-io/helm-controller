package main

import (
	"github.com/rancher/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
	batch "k8s.io/api/batch/v1"
	rbac "k8s.io/api/rbac/v1"
	core "k8s.io/api/core/v1"
)

func main() {
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/helm-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"helm.cattle.io": {
				Types: []interface{}{
					v1.HelmChart{},
				},
				GenerateTypes: true,
			},
			"batch": {
				Types: []interface{}{
					batch.Job{},
				},
				InformersPackage: "k8s.io/client-go/informers",
				ClientSetPackage: "k8s.io/client-go/kubernetes",
				ListersPackage:   "k8s.io/client-go/listers",
			},
			"rbac": {
				Types: []interface{}{
					rbac.ClusterRoleBinding{},
				},
				InformersPackage: "k8s.io/client-go/informers",
				ClientSetPackage: "k8s.io/client-go/kubernetes",
				ListersPackage:   "k8s.io/client-go/listers",
			},
			"core": {
				Types: []interface{}{
					core.ServiceAccount{},
				},
				InformersPackage: "k8s.io/client-go/informers",
				ClientSetPackage: "k8s.io/client-go/kubernetes",
				ListersPackage:   "k8s.io/client-go/listers",
			},
		},
	})
}