package state

import (
	"sort"
)

// DiffType は差分の種類を表す
type DiffType string

const (
	DiffTypeAdded      DiffType = "added"
	DiffTypeModified   DiffType = "modified"
	DiffTypeRemoved    DiffType = "removed"
	DiffTypeAlwaysSync DiffType = "always-sync"
)

// DiffEntry は個別リソースの差分情報を表す
type DiffEntry struct {
	Key      string   // ステートキー文字列
	DiffType DiffType // 差分の種類
	OldHash  string   // removed/modified時の旧ハッシュ
	NewHash  string   // added/modified/always-sync時の新ハッシュ
}

// ComputeDiff は既存ステートと現在のエントリを比較して差分を算出する。
// alwaysSyncKeys に含まれるキーは always-sync として扱う。
func ComputeDiff(stateData *StateData, currentEntries map[string]StateEntry, alwaysSyncKeys map[string]bool) []DiffEntry {
	var entries []DiffEntry

	// 現在のエントリを走査: added / modified / always-sync
	for key, current := range currentEntries {
		if alwaysSyncKeys[key] {
			entries = append(entries, DiffEntry{
				Key:      key,
				DiffType: DiffTypeAlwaysSync,
				NewHash:  current.ContentHash,
			})
			continue
		}

		old, exists := stateData.Entries[key]
		if !exists {
			entries = append(entries, DiffEntry{
				Key:      key,
				DiffType: DiffTypeAdded,
				NewHash:  current.ContentHash,
			})
		} else if old.ContentHash != current.ContentHash {
			entries = append(entries, DiffEntry{
				Key:      key,
				DiffType: DiffTypeModified,
				OldHash:  old.ContentHash,
				NewHash:  current.ContentHash,
			})
		}
	}

	// 既存ステートにあって現在のエントリにないもの: removed
	for key, old := range stateData.Entries {
		if _, exists := currentEntries[key]; !exists {
			entries = append(entries, DiffEntry{
				Key:      key,
				DiffType: DiffTypeRemoved,
				OldHash:  old.ContentHash,
			})
		}
	}

	// 安定したソート: DiffType → Key の順
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DiffType != entries[j].DiffType {
			return diffTypeOrder(entries[i].DiffType) < diffTypeOrder(entries[j].DiffType)
		}
		return entries[i].Key < entries[j].Key
	})

	return entries
}

func diffTypeOrder(t DiffType) int {
	switch t {
	case DiffTypeAdded:
		return 0
	case DiffTypeModified:
		return 1
	case DiffTypeRemoved:
		return 2
	case DiffTypeAlwaysSync:
		return 3
	default:
		return 4
	}
}
