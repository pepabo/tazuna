package manifest

import (
	"bytes"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// convertManifestsToObjects は複数の定義が入ったマニフェスト群を解析し、
// Kubernetes Clientが扱えるclient.Object型に変換します
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

	lines := bytes.Split(b, []byte("\n"))

	tripleHyphenIndices := map[int]bool{}

	for i := range lines {
		if bytes.HasPrefix(lines[i], []byte("---")) {
			tripleHyphenIndices[i] = true
		}
	}

	bufferLines := [][]byte{}
	manifestBytes := [][]byte{}
	for i := range lines {
		if _, ok := tripleHyphenIndices[i]; ok {
			manifestBytes = append(manifestBytes, bytes.Join(bufferLines, []byte("\n")))
			// マニフェストの境目
			bufferLines = [][]byte{}
			continue
		}

		bufferLines = append(bufferLines, lines[i])
	}

	if len(bufferLines) > 0 {
		manifestBytes = append(manifestBytes, bytes.Join(bufferLines, []byte("\n")))
	}

	for _, manifest := range manifestBytes {
		// helm templateなど、 --- から始まるマニフェスト群を生成する場合空があり得るので無視します
		// ingress-nginxなどでは、Deprecatedになっていて中身がコメントだけのyamlも生成されうるので、
		// kind: が入っていないものをすべて無視することにします。
		if !bytes.Contains(manifest, []byte("kind:")) {
			continue
		}

		var data map[string]interface{}
		if err := yaml.Unmarshal(manifest, &data); err != nil {
			return nil, err
		}

		// kind: List の場合は items を展開する
		if data["kind"] == "List" {
			items, ok := data["items"].([]interface{})
			if ok {
				for _, item := range items {
					itemData, ok := item.(map[string]interface{})
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
func setNamespaceIfEmpty(data map[string]interface{}, namespace string) {
	if data["metadata"] == nil {
		data["metadata"] = map[string]interface{}{}
	}
	if data["metadata"].(map[string]interface{})["namespace"] == nil && namespace != "" {
		data["metadata"].(map[string]interface{})["namespace"] = namespace
	}
}
