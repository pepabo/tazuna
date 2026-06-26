package manager

import (
	"context"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager interface {
	// Apply はマニフェストをレンダリングしてクラスタへ適用し、適用対象となった
	// render 済みオブジェクト群を返します。呼び出し側 (runner) はこの戻り値を
	// state hash 計算・保存に再利用することで、apply 後の再レンダリング (二重 render)
	// を避けられます。
	//
	// 返すオブジェクトは hash 計算のため *unstructured.Unstructured であり、
	// 同一マニフェストに対する Build() の出力を ConvertManifestsToObjects したものと
	// 同一になることを各実装が保証します (state diff/sync との整合性のため)。
	Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) ([]client.Object, error)
	Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error
	Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error)
}
