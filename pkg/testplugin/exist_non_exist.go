package testplugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExistNonExist struct {
	client client.Client
}

// Run implements Plugin.
func (e *ExistNonExist) Run(
	ctx context.Context,
	logger *slog.Logger,
	spec *v1.TestPluginSpec,
) error {
	args := spec.ExistNonExist
	if args == nil {
		return fmt.Errorf(".spec.existNonExist is not defined")
	}

	gv, err := schema.ParseGroupVersion(args.Resource.APIVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	gvk := gv.WithKind(args.Resource.Kind)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := client.ObjectKey{
		Namespace: args.Namespace,
		Name:      args.Name,
	}
	err = e.client.Get(ctx, key, obj)

	if args.ShouldExist {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("resource not found (resource: %s %s/%s)", gvk.Kind, args.Namespace, args.Name)
		}
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// shouldExist: false
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return fmt.Errorf("resource exists (resource: %s %s/%s)", gvk.Kind, args.Namespace, args.Name)
}

// Type implements Plugin.
func (e *ExistNonExist) Type() v1.TestPluginType {
	return v1.TestPluginTypeExistNonExist
}

func NewExistNonExist(c client.Client) *ExistNonExist {
	return &ExistNonExist{c}
}

var _ Plugin = &ExistNonExist{}
