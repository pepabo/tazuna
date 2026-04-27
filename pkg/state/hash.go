package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ComputeContentHash はunstructured.UnstructuredオブジェクトからSHA-256コンテンツハッシュを算出する。
// server-side付与のフィールド（resourceVersion, uid等）とstatusは除外してハッシュを計算する。
func ComputeContentHash(obj *unstructured.Unstructured) (string, error) {
	// ディープコピーしてオリジナルを変更しない
	// NOTE: obj.DeepCopy()はint64値でpanicするため、JSON経由でコピーする
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object for copying: %w", err)
	}
	var copied map[string]interface{}
	if err := json.Unmarshal(raw, &copied); err != nil {
		return "", fmt.Errorf("failed to unmarshal object for copying: %w", err)
	}

	// server-side付与のmetadataフィールドを除外
	if metadata, ok := copied["metadata"].(map[string]interface{}); ok {
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")
		delete(metadata, "creationTimestamp")
		delete(metadata, "generation")
		delete(metadata, "managedFields")
		delete(metadata, "selfLink")
	}

	// statusフィールドを除外
	delete(copied, "status")

	data, err := json.Marshal(copied)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object for hashing: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
