package resource

import (
	"context"
	"log/slog"
	"time"

	"github.com/cockroachdb/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateForObject は既存リソースがあればUpdate、なければCreateを行う。
// JobやPodなどimmutableなフィールドを持つリソースはDelete→Createで対応する。
func CreateOrUpdateForObject(
	ctx context.Context,
	c client.Client,
	obj client.Object,
) error {
	dummy := &unstructured.Unstructured{}
	dummy.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	err := c.Get(ctx, key, dummy)
	if err == nil {
		// JobはimmutableなフィールドがありUpdateできないため、Delete→Createで対応する
		// Podもspec内のほとんどのフィールドがimmutableなため同様に対応する
		gvk := obj.GetObjectKind().GroupVersionKind()
		if (gvk.Group == "batch" && gvk.Kind == "Job") || (gvk.Group == "" && gvk.Kind == "Pod") {
			if err := c.Delete(ctx, dummy); err != nil {
				return errors.WithStack(err)
			}
			slog.InfoContext(ctx, "waiting for deletion to complete",
				slog.String("namespace", obj.GetNamespace()),
				slog.String("name", obj.GetName()),
				slog.String("kind", gvk.Kind))
			if err := WaitForDeletion(ctx, c, dummy); err != nil {
				return errors.WithStack(err)
			}
			if err := c.Create(ctx, obj); err != nil {
				return errors.WithStack(err)
			}
			return nil
		}

		obj.SetResourceVersion(dummy.GetResourceVersion())
		if err := c.Update(ctx, obj); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, obj); err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	// apierrors.IsNotFound() 以外のget errorを返すためerrを返す
	return errors.WithStack(err)
}

// WaitForDeletion はリソースが削除されるまでポーリングして待機する。
func WaitForDeletion(ctx context.Context, c client.Client, obj client.Object) error {
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	dummy := &unstructured.Unstructured{}
	dummy.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case <-ticker.C:
			err := c.Get(ctx, key, dummy)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return errors.WithStack(err)
			}
			slog.InfoContext(ctx, "resource still exists, waiting for deletion",
				slog.String("namespace", obj.GetNamespace()),
				slog.String("name", obj.GetName()))
		}
	}
}

// DeleteObject はリソースを削除する。NotFoundの場合は無視する。
func DeleteObject(ctx context.Context, c client.Client, obj client.Object) error {
	if err := c.Delete(ctx, obj); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
