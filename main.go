//go:generate go run pkg/codegen/cleanup/main.go
//go:generate rm -rf pkg/generated pkg/crds/yaml/generated
//go:generate go run pkg/codegen/main.go
//go:generate controller-gen crd:generateEmbeddedObjectMeta=true paths=./pkg/apis/... output:crd:dir=./pkg/crds/yaml/generated
//go:generate crd-ref-docs --config=crd-ref-docs.yaml --renderer=markdown --output-path=doc/helmchart.md

package main

import (
	"context"
	"errors"
	_ "net/http/pprof"
	"os"

	"github.com/k3s-io/helm-controller/pkg/app"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/apiextensions.k8s.io"
	_ "github.com/rancher/wrangler/v3/pkg/generated/controllers/networking.k8s.io"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
)

func main() {
	app := app.New()
	ctx := signals.SetupSignalContext()
	if err := app.RunContext(ctx, os.Args); err != nil && !errors.Is(err, context.Canceled) {
		logrus.Fatal(err)
	}
}
