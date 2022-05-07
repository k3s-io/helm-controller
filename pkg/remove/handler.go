package remove

import (
	"context"

	"github.com/rancher/wrangler/pkg/generic"
	"k8s.io/apimachinery/pkg/runtime"
)

// Controller is an interface that allows the ScopedOnRemoveHandler to register a generic RemoveHandler
type Controller interface {
	AddGenericHandler(ctx context.Context, name string, handler generic.Handler)
	Updater() generic.Updater
}

// ScopeFunc is a function that determines whether the ScopedOnRemoveHandler should manage the lifecycle of the given object
type ScopeFunc func(key string, obj runtime.Object) (bool, error)

// RegisterScopedOnRemoveHandler registers a handler that does the same thing as an OnRemove handler but only applies finalizers or sync logic
// to objects that pass the provided scopeFunc; this ensures that finalizers are not added to all resources across an entire cluster but are
// instead only scoped to resources that this controller is meant to watch.
//
// TODO: move this to rancher/wrangler as a generic construct to be used across multiple controllers as part of the auto-generated code
func RegisterScopedOnRemoveHandler(ctx context.Context, controller Controller, name string, scopeFunc ScopeFunc, handler generic.Handler) {
	onRemove := generic.NewRemoveHandler(name, controller.Updater(), handler)
	controller.AddGenericHandler(ctx, name, func(key string, obj runtime.Object) (runtime.Object, error) {
		if obj == nil {
			return nil, nil
		}
		isScoped, err := scopeFunc(key, obj)
		if err != nil {
			return obj, err
		}
		if !isScoped {
			return obj, nil
		}
		return onRemove(key, obj)
	})
}
