package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TazunaNamespace はステートConfigMapを格納するnamespace
	TazunaNamespace = "tazuna"
	// configMapPrefix はステートConfigMap名のプレフィックス
	configMapPrefix = "tazuna-state-"
	// metadataKey はConfigMap内のメタデータ用予約キー
	metadataKey = "_metadata"
	// stateKeyEncodedSep はStateKey文字列内の '/' をConfigMap data keyに収めるための置換文字列。
	// ConfigMapのdata keyは [-._a-zA-Z0-9]+ しか許さず、'/' をそのまま使えない。
	// k8s DNS-1123 名 (manifest名/group/namespace/name) はいずれも '_' を含まないため、
	// '__' を区切りマーカとして用いれば安全に往復変換できる。
	stateKeyEncodedSep = "__"
)

// encodeStateKey はStateKey.String() 形式の文字列をConfigMap data keyに使える形式へ変換する。
func encodeStateKey(k string) string {
	return strings.ReplaceAll(k, "/", stateKeyEncodedSep)
}

// decodeStateKey は encodeStateKey の逆変換を行う。
func decodeStateKey(k string) string {
	return strings.ReplaceAll(k, stateKeyEncodedSep, "/")
}

// ConfigMapStateStore はConfigMapベースのStateStore実装
type ConfigMapStateStore struct {
	client client.Client
}

// NewConfigMapStateStore はConfigMapStateStoreを生成する
func NewConfigMapStateStore(c client.Client) *ConfigMapStateStore {
	return &ConfigMapStateStore{client: c}
}

// ConfigMapName はmanifest名からConfigMap名を生成する
func ConfigMapName(manifestName string) string {
	return fmt.Sprintf("%s%s", configMapPrefix, manifestName)
}

// Get はmanifest名に対応するステートをConfigMapから読み込む。
// ConfigMapが存在しない場合は空のStateDataを返す。
func (s *ConfigMapStateStore) Get(ctx context.Context, manifestName string) (*StateData, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Namespace: TazunaNamespace,
		Name:      ConfigMapName(manifestName),
	}

	if err := s.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return &StateData{
				Entries: make(map[string]StateEntry),
			}, nil
		}
		return nil, errors.Wrapf(err, "failed to get state ConfigMap %s", key.Name)
	}

	return parseConfigMapData(cm.Data)
}

// Save はmanifest名に対応するステートをConfigMapに保存する。
// ConfigMapが存在しない場合は新規作成、存在する場合は更新する。
func (s *ConfigMapStateStore) Save(ctx context.Context, manifestName string, data *StateData) error {
	cmData, err := buildConfigMapData(data)
	if err != nil {
		return err
	}

	cmName := ConfigMapName(manifestName)
	key := types.NamespacedName{
		Namespace: TazunaNamespace,
		Name:      cmName,
	}

	existing := &corev1.ConfigMap{}
	err = s.client.Get(ctx, key, existing)
	if err == nil {
		existing.Data = cmData
		if err := s.client.Update(ctx, existing); err != nil {
			return errors.Wrapf(err, "failed to update state ConfigMap %s", cmName)
		}
		return nil
	}

	if apierrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: TazunaNamespace,
			},
			Data: cmData,
		}
		if err := s.client.Create(ctx, cm); err != nil {
			return errors.Wrapf(err, "failed to create state ConfigMap %s", cmName)
		}
		return nil
	}

	return errors.Wrapf(err, "failed to get state ConfigMap %s for save", cmName)
}

// parseConfigMapData はConfigMapのdataフィールドからStateDataをパースする
func parseConfigMapData(data map[string]string) (*StateData, error) {
	sd := &StateData{
		Entries: make(map[string]StateEntry),
	}

	for k, v := range data {
		if k == metadataKey {
			var meta StateMetadata
			if err := json.Unmarshal([]byte(v), &meta); err != nil {
				return nil, errors.Wrapf(err, "failed to parse state metadata")
			}
			sd.Metadata = meta
			continue
		}

		var entry StateEntry
		if err := json.Unmarshal([]byte(v), &entry); err != nil {
			return nil, errors.Wrapf(err, "failed to parse state entry for key %q", k)
		}
		sd.Entries[decodeStateKey(k)] = entry
	}

	return sd, nil
}

// buildConfigMapData はStateDataからConfigMap用のdataマップを構築する
func buildConfigMapData(data *StateData) (map[string]string, error) {
	cmData := make(map[string]string)

	metaJSON, err := json.Marshal(data.Metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal state metadata")
	}
	cmData[metadataKey] = string(metaJSON)

	for k, entry := range data.Entries {
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal state entry for key %q", k)
		}
		cmData[encodeStateKey(k)] = string(entryJSON)
	}

	return cmData, nil
}
