package state

import (
	"context"
	"log/slog"

	"github.com/cockroachdb/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureNamespace はtazuna namespaceが存在することを保証する。
// 存在しない場合は新規作成し、既に存在する場合は何もしない。
func EnsureNamespace(ctx context.Context, c client.Client) error {
	ns := &corev1.Namespace{}
	err := c.Get(ctx, client.ObjectKey{Name: TazunaNamespace}, ns)
	if err == nil {
		slog.Debug("namespace already exists", "namespace", TazunaNamespace)
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get namespace %s", TazunaNamespace)
	}

	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: TazunaNamespace,
		},
	}
	if err := c.Create(ctx, ns); err != nil {
		return errors.Wrapf(err, "failed to create namespace %s", TazunaNamespace)
	}

	slog.Info("created namespace", "namespace", TazunaNamespace)
	return nil
}
