package main

import (
	"os"

	"github.com/k3s-io/helm-controller/pkg/crd"
	wcrd "github.com/rancher/wrangler/pkg/crd"
)

func main() {
	wcrd.Print(os.Stdout, crd.List())
}
