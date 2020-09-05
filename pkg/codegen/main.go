package main

import (
	"os"

	v1 "github.com/rancher/helm-controller/pkg/apis/helm.cattle.io/v1"
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/helm-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
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
