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

// fieldManager は Server-Side Apply の FieldOwner 名。
// この名前で managedFields に所有権が記録される。
const fieldManager = "tazuna"

// CreateOrUpdateForObject はリソースを Server-Side Apply (SSA) で適用する。
//
// 以前は Get してから ResourceVersion を引き継いだ full overwrite Update を行っていたが、
// それではコントローラやデフォルティングが設定したフィールドまで踏み潰してしまう。
// SSA + FieldOwner("tazuna") に切り替えることで、tazuna が宣言したフィールドのみを所有し、
// 他者が管理するフィールドを保持できる。ForceOwnership により、過去に別マネージャ
// (kubectl など) が所有していたフィールドも tazuna が引き取る (bootstrap 用途として妥当)。
//
// JobやPodなどspecの大半がimmutableなリソースはSSAでも衝突するため、従来どおり
// Delete→Createで対応する。
func CreateOrUpdateForObject(
	ctx context.Context,
	c client.Client,
	obj client.Object,
) error {
	gvk := obj.GetObjectKind().GroupVersionKind()

	// JobはimmutableなフィールドがありSSAでも衝突するため、Delete→Createで対応する。
	// Podもspec内のほとんどのフィールドがimmutableなため同様に対応する。
	if (gvk.Group == "batch" && gvk.Kind == "Job") || (gvk.Group == "" && gvk.Kind == "Pod") {
		return recreateImmutableObject(ctx, c, obj)
	}

	// SSA は desired を直接 Apply する。tazuna は任意の CRD を unstructured で扱うため、
	// ApplyConfigurationFromUnstructured で動的に ApplyConfiguration を構築する。
	// managedFields / resourceVersion は SSA では不要 (含めると衝突の原因になる) なので
	// 落としてから適用する。
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return errors.Errorf("server-side apply requires *unstructured.Unstructured, got %T", obj)
	}
	applyObj := u.DeepCopy()
	applyObj.SetResourceVersion("")
	applyObj.SetManagedFields(nil)

	if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(applyObj), client.FieldOwner(fieldManager), client.ForceOwnership); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// recreateImmutableObject は immutable なリソース (Job/Pod) を Delete→Create で再適用する。
func recreateImmutableObject(ctx context.Context, c client.Client, obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	dummy := &unstructured.Unstructured{}
	dummy.SetGroupVersionKind(gvk)
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	err := c.Get(ctx, key, dummy)
	if err == nil {
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

	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, obj); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// apierrors.IsNotFound() 以外のget errorを返すためerrを返す
	return errors.WithStack(err)
}

// deletionTimeout は WaitForDeletion の上限タイムアウト。
// finalizer が詰まったリソースがあっても apply が無期限にハングしないようにする。
const deletionTimeout = 5 * time.Minute

// WaitForDeletion はリソースが削除されるまでポーリングして待機する。
// 上限タイムアウト (5 分) を超えるとエラーを返す。
func WaitForDeletion(ctx context.Context, c client.Client, obj client.Object) error {
	return waitForDeletion(ctx, c, obj, deletionTimeout)
}

func waitForDeletion(ctx context.Context, c client.Client, obj client.Object, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
			return errors.Wrapf(ctx.Err(), "giving up waiting for deletion of %s/%s (a stuck finalizer may be blocking it)",
				obj.GetNamespace(), obj.GetName())
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
