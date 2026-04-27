package manager

import (
	"context"
	"log/slog"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type Kustomize struct {
	client client.Client
}

func NewKustomize(client client.Client) *Kustomize {
	return &Kustomize{client}
}

// Destroy implements Manager.
func (k *Kustomize) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	fs := filesys.MakeFsOnDisk()
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resourceMap, err := kustomizer.Run(fs, m.Path)
	if err != nil {
		return errors.WithStack(err)
	}

	out, err := resourceMap.AsYaml()
	if err != nil {
		return errors.WithStack(err)
	}

	if m.Kustomize == nil {
		m.Kustomize = &v1.ManifestKustomize{}
	}

	objects, err := manifest.ConvertManifestsToObjects(out, m.Kustomize.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}
	logger.DebugContext(ctx, "successfully converted manifests to objects", slog.Int("count", len(objects)))

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to delete an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.DeleteObject(ctx, k.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Apply implements Manager.
func (k *Kustomize) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	fs := filesys.MakeFsOnDisk()
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resourceMap, err := kustomizer.Run(fs, m.Path)
	if err != nil {
		return errors.WithStack(err)
	}

	out, err := resourceMap.AsYaml()
	if err != nil {
		return errors.WithStack(err)
	}

	if m.Kustomize == nil {
		m.Kustomize = &v1.ManifestKustomize{}
	}

	objects, err := manifest.ConvertManifestsToObjects(out, m.Kustomize.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}
	logger.DebugContext(ctx, "successfully converted manifests to objects", slog.Int("count", len(objects)))

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to create or update an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()))
		if err := resource.CreateOrUpdateForObject(ctx, k.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

var _ Manager = &Kustomize{}

// Build implements Manager.
func (k *Kustomize) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error) {
	fs := filesys.MakeFsOnDisk()
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resourceMap, err := kustomizer.Run(fs, m.Path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	out, err := resourceMap.AsYaml()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(out), nil
}
