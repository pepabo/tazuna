package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildCmd_WorksWithoutKubeconfig は kubeconfig のない環境 (CI 等) でも
// tazuna build が実行できることを確認する。build はクラスタを変更しないため
// kubeconfig は必須ではない。
func TestBuildCmd_WorksWithoutKubeconfig(t *testing.T) {
	// kubeconfig を確実に見つからなくする
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	dir := t.TempDir()
	kustomizeDir := filepath.Join(dir, "kustomize")
	if err := os.MkdirAll(kustomizeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kustomizeDir, "configmap.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: build-test
  namespace: default
data:
  key: value
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kustomizeDir, "kustomization.yaml"), []byte(`resources:
  - configmap.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - name: kustomize-app
    type: kustomize
    path: ./kustomize
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resetCheckFlags(t)
	buildCmd.SetOut(io.Discard)
	buildCmd.SetErr(io.Discard)
	if err := buildCmd.ParseFlags([]string{"-f", path}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	buildCmd.SetContext(context.Background())
	if err := buildCmd.RunE(buildCmd, []string{}); err != nil {
		t.Fatalf("build without kubeconfig returned error: %v", err)
	}
}
