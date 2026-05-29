package validator

import (
	"fmt"
	"path/filepath"
	"strings"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// FixManifestNames はname未設定のmanifestに対してtypeとpathから名前を自動付与する。
// 既存の名前との重複を回避するためにサフィックスを付与する。
func FixManifestNames(manifests []v1.Manifest) {
	usedNames := collectExistingNames(manifests)
	for i := range manifests {
		if manifests[i].Name == "" {
			name := generateName(manifests[i])
			name = ensureUnique(name, usedNames)
			manifests[i].Name = name
			usedNames[name] = true
		}
	}
}

func collectExistingNames(manifests []v1.Manifest) map[string]bool {
	names := make(map[string]bool)
	for _, m := range manifests {
		if m.Name != "" {
			names[m.Name] = true
		}
	}
	return names
}

// generateName はtypeとpathからmanifest名を生成する。
func generateName(m v1.Manifest) string {
	typeName := string(m.Type)
	if typeName == "" {
		typeName = "manifest"
	}

	dirName := extractDirName(m.Path)
	if dirName == "" {
		return typeName
	}

	return typeName + "-" + dirName
}

// extractDirName はpathから末尾のディレクトリ名を取得し、名前として使える形式に変換する。
func extractDirName(path string) string {
	if path == "" || path == "." {
		return ""
	}

	// 末尾のスラッシュを除去してからBase
	cleaned := filepath.Clean(path)
	base := filepath.Base(cleaned)

	if base == "." || base == "/" {
		return ""
	}

	// 名前に使えない文字を置換
	name := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, base)

	return strings.Trim(name, "-")
}

// ensureUnique は名前が既に使用されている場合にサフィックスを付与して一意にする。
func ensureUnique(name string, usedNames map[string]bool) string {
	if !usedNames[name] {
		return name
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !usedNames[candidate] {
			return candidate
		}
	}
}
