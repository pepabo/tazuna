package manifest

import (
	"bytes"
	"errors"
	"io"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConvertManifestsToObjects は複数の定義が入ったマニフェスト群を解析し、
// Kubernetes Clientが扱えるclient.Object型に変換します。
//
// ドキュメント分割は k8s.io/apimachinery の YAMLOrJSONDecoder を用います。
// 以前は bytes.Split で行頭 `---` を手書き検出していましたが、これは複数行
// ブロックスカラー中に出現する `---` や `kind:` という名前のフィールドで誤動作し得たため、
// YAML ドキュメントストリームを正しく解釈するデコーダへ置き換えています。
//
// NOTE: schemeとUniversalDeserializerを利用した型安全なUnmarshalを検討しましたが、
//
//	Tazunaとしてはリソースがなんであるかに関心を持たず、なんであれapplyするだけなので、
//	unstructuredを利用します。
//	これにより、任意のCRDを拡張性を持ってサポートできます。
//
// 第二引数のnamespaceは、
// helm templateなどでnamespaceが指定されていない場合に、
// そのnamespaceを設定するために利用します。
func ConvertManifestsToObjects(
	b []byte,
	namespace string,
) ([]client.Object, error) {
	objects := []client.Object{}

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(b), 4096)
	for {
		var data map[string]any
		if err := decoder.Decode(&data); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		// 空ドキュメント（`---` のみ、コメントのみ、空行のみ）は nil になるので無視する。
		if data == nil {
			continue
		}

		// helm template などでは Deprecated になっていて中身がコメントだけの yaml も
		// 生成されうるので、kind を持たないドキュメントはすべて無視する。
		if _, ok := data["kind"]; !ok {
			continue
		}

		// kind: List の場合は items を展開する
		if data["kind"] == "List" {
			items, ok := data["items"].([]any)
			if ok {
				for _, item := range items {
					itemData, ok := item.(map[string]any)
					if !ok {
						continue
					}
					setNamespaceIfEmpty(itemData, namespace)
					obj := unstructured.Unstructured{Object: itemData}
					objects = append(objects, &obj)
				}
				continue
			}
		}

		setNamespaceIfEmpty(data, namespace)
		obj := unstructured.Unstructured{Object: data}
		objects = append(objects, &obj)
	}

	crdObjects := []client.Object{}
	nonCRDObjects := []client.Object{}

	for _, obj := range objects {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.GroupKind().Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
			crdObjects = append(crdObjects, obj)
			continue
		}

		nonCRDObjects = append(nonCRDObjects, obj)
	}

	return append(crdObjects, nonCRDObjects...), nil
}

// setNamespaceIfEmptyは、metadata.namespaceが未設定の場合にnamespaceを設定します。
// NOTE: falcoなどのマニフェストでは、roleなどmetadata.namespaceが空のものが存在する場合がある
func setNamespaceIfEmpty(data map[string]any, namespace string) {
	if data["metadata"] == nil {
		data["metadata"] = map[string]any{}
	}
	if data["metadata"].(map[string]any)["namespace"] == nil && namespace != "" {
		data["metadata"].(map[string]any)["namespace"] = namespace
	}
}
