package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

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
	// '_' 始まりのシーケンスをエスケープマーカとして安全に往復変換できる。
	// '/' は頻出するため短い '__' を維持する (既存 state との後方互換)。
	stateKeyEncodedSep = "__"
)

// encodeStateKey はStateKey.String() 形式の文字列をConfigMap data keyに使える形式へ変換する。
//   - '/' → "__" (従来どおり)
//   - その他 [-._a-zA-Z0-9] に含まれないバイト → "_x" + 16進2桁 (小文字)
//
// RBAC リソース名は "cert-manager-webhook:subjectaccessreviews" のように ':' を
// 含むことがあり、'/' 置換だけでは ConfigMap data key の制約に違反して
// Save が失敗するため、任意の不許可文字を可逆にエスケープする。
func encodeStateKey(k string) string {
	var b strings.Builder
	b.Grow(len(k))
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c == '/':
			b.WriteString(stateKeyEncodedSep)
		case c == '-' || c == '.' ||
			('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9'):
			b.WriteByte(c)
		default:
			// '_' 自体もここに含まれる (raw 名に現れない前提だが、現れても可逆になる)
			fmt.Fprintf(&b, "_x%02x", c)
		}
	}
	return b.String()
}

// decodeStateKey は encodeStateKey の逆変換を行う。
// 左から走査し、'_' に続く文字でエスケープ種別を判定する:
//   - "__"   → '/'
//   - "_xHH" → 16進2桁のバイト
//
// 想定外の '_' シーケンス (旧バージョンや手書きの key) はそのまま残す。
func decodeStateKey(k string) string {
	var b strings.Builder
	b.Grow(len(k))
	for i := 0; i < len(k); {
		if k[i] != '_' {
			b.WriteByte(k[i])
			i++
			continue
		}
		if i+1 < len(k) && k[i+1] == '_' {
			b.WriteByte('/')
			i += 2
			continue
		}
		if i+3 < len(k) && k[i+1] == 'x' {
			if v, err := strconv.ParseUint(k[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 4
				continue
			}
		}
		b.WriteByte(k[i])
		i++
	}
	return b.String()
}

// ConfigMapStateStore はConfigMapベースのStateStore実装
type ConfigMapStateStore struct {
	client client.Client

	// nsMu / nsEnsured はSaveごとのnamespace存在確認GETを同一store内で
	// 1回に抑えるためのフィールド。失敗時はキャッシュせず次回も再試行する。
	nsMu      sync.Mutex
	nsEnsured bool
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
// state ConfigMap は tazuna namespace 配下に作成されるため、書き込み前に
// namespace の存在を保証する (呼び出し側での ensure 忘れを防ぐ)。
func (s *ConfigMapStateStore) Save(ctx context.Context, manifestName string, data *StateData) error {
	if err := s.ensureNamespaceOnce(ctx); err != nil {
		return errors.Wrapf(err, "failed to ensure %s namespace before saving state", TazunaNamespace)
	}

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

// ensureNamespaceOnce は同一store内でnamespaceのensureを成功するまで1回に抑える。
// 成功した後はGETを発行しない。失敗はキャッシュしないため次回のSaveで再試行される。
func (s *ConfigMapStateStore) ensureNamespaceOnce(ctx context.Context) error {
	s.nsMu.Lock()
	defer s.nsMu.Unlock()
	if s.nsEnsured {
		return nil
	}
	if err := EnsureNamespace(ctx, s.client); err != nil {
		return err
	}
	s.nsEnsured = true
	return nil
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
