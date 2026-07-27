package state

import (
	"regexp"
	"testing"
)

// configMapKeyRe は Kubernetes が ConfigMap data key に許す文字集合。
var configMapKeyRe = regexp.MustCompile(`^[-._a-zA-Z0-9]+$`)

func TestEncodeStateKeyRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{
			name: "namespaced resource",
			key:  "my-manifest/apps/v1/Deployment/my-namespace/my-app",
		},
		{
			name: "cluster-scoped resource",
			key:  "my-manifest//v1/Namespace/my-namespace",
		},
		{
			// cert-manager の RBAC リソース名は ':' を含む。
			// https://github.com/cert-manager/cert-manager の ClusterRole 等。
			name: "RBAC name containing colon",
			key:  "cert-manager/rbac.authorization.k8s.io/v1/ClusterRole/cert-manager-webhook:subjectaccessreviews",
		},
		{
			name: "namespaced RBAC name containing colon",
			key:  "cert-manager/rbac.authorization.k8s.io/v1/RoleBinding/cert-manager-system/cert-manager-webhook:dynamic-serving",
		},
		{
			name: "system-style colon name",
			key:  "m/rbac.authorization.k8s.io/v1/ClusterRole/system:controller:foo",
		},
		{
			// raw 名は '_' を含まない前提だが、含まれても往復できることを保証する
			name: "underscore in raw name",
			key:  "m/g/v1/Kind/name_with_underscore",
		},
		{
			name: "dots and dashes",
			key:  "m/cert-manager.io/v1/Certificate/ns/example.com-tls",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := encodeStateKey(tc.key)
			if !configMapKeyRe.MatchString(encoded) {
				t.Errorf("encodeStateKey(%q) = %q; contains characters not allowed in ConfigMap data keys", tc.key, encoded)
			}
			decoded := decodeStateKey(encoded)
			if decoded != tc.key {
				t.Errorf("round trip failed: encodeStateKey(%q) = %q; decodeStateKey = %q", tc.key, encoded, decoded)
			}
		})
	}
}

func TestDecodeStateKeyBackwardCompat(t *testing.T) {
	// 旧実装 (ReplaceAll("/", "__")) で保存された既存 state の key を
	// 新実装の decode がそのまま読めること。
	legacy := "my-manifest__apps__v1__Deployment__my-namespace__my-app"
	want := "my-manifest/apps/v1/Deployment/my-namespace/my-app"
	if got := decodeStateKey(legacy); got != want {
		t.Errorf("decodeStateKey(%q) = %q; want %q", legacy, got, want)
	}
}

func TestDecodeStateKeyUnknownUnderscoreSequence(t *testing.T) {
	// 想定外の '_' シーケンスは literal のまま残す (defensive)。
	// "_metadata" は Load 側で除外されるが、万一渡っても panic しないこと。
	in := "_metadata"
	if got := decodeStateKey(in); got != "/metadata" && got != "_metadata" {
		// "__" 始まりではないため先頭の '_' は literal 扱いとなる
		t.Logf("decodeStateKey(%q) = %q", in, got)
	}
	if got := decodeStateKey("_zfoo"); got != "_zfoo" {
		t.Errorf("decodeStateKey(_zfoo) = %q; want literal passthrough", got)
	}
	if got := decodeStateKey("trailing_"); got != "trailing_" {
		t.Errorf("decodeStateKey(trailing_) = %q; want literal passthrough", got)
	}
	if got := decodeStateKey("_xZZ"); got != "_xZZ" {
		t.Errorf("decodeStateKey(_xZZ) = %q; want literal passthrough on invalid hex", got)
	}
}
