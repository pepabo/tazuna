package state

import (
	"context"
)

// StateEntry はステートに保存される個別リソースのエントリ
type StateEntry struct {
	ContentHash string `json:"contentHash"`
}

// StateMetadata はステートConfigMapのメタデータ (_metadata キー)
type StateMetadata struct {
	GitCommitHash string `json:"gitCommitHash"`
	LastSyncedAt  string `json:"lastSyncedAt"`
}

// StateData はmanifest単位のステート全体を表す
type StateData struct {
	Metadata StateMetadata
	Entries  map[string]StateEntry // キー: ステートキー文字列
}

// StateStore はステートの読み書きインターフェース
type StateStore interface {
	// Get はmanifest名に対応するステートを取得する。
	// ConfigMapが存在しない場合は空のStateDataを返す。
	Get(ctx context.Context, manifestName string) (*StateData, error)
	// Save はmanifest名に対応するステートを保存する。
	Save(ctx context.Context, manifestName string, data *StateData) error
}
