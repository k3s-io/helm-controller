package main

import (
	"os"

	"github.com/k3s-io/helm-controller/pkg/crd"
	_ "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	wcrd "github.com/rancher/wrangler/v2/pkg/crd"
)

func main() {
	wcrd.Print(os.Stdout, crd.List())
}
