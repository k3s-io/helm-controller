package main

import (
	"os"

	v1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"

	controllergen "github.com/rancher/wrangler/v3/pkg/controller-gen"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/k3s-io/helm-controller/pkg/generated",
		Boilerplate:   "scripts/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"helm.cattle.io": {
				Types: []any{
					v1.HelmChart{},
					v1.HelmChartConfig{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
		},
	})
}
