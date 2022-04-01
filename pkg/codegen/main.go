package main

import (
	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
)

func main() {
	controllergen.Run(args.Options{
		OutputPackage: "github.com/k3s-io/helm-controller/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"helm.cattle.io": {
				Types: []interface{}{
					v1.HelmChart{},
					v1.HelmChartConfig{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
		},
	})
}
