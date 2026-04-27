package state

import (
	"testing"
)

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name           string
		stateData      *StateData
		currentEntries map[string]StateEntry
		alwaysSyncKeys map[string]bool
		expected       []DiffEntry
	}{
		{
			name: "added: new resource",
			stateData: &StateData{
				Entries: map[string]StateEntry{},
			},
			currentEntries: map[string]StateEntry{
				"m1//v1/ConfigMap/default/config": {ContentHash: "abc123"},
			},
			alwaysSyncKeys: nil,
			expected: []DiffEntry{
				{Key: "m1//v1/ConfigMap/default/config", DiffType: DiffTypeAdded, NewHash: "abc123"},
			},
		},
		{
			name: "modified: hashes differ",
			stateData: &StateData{
				Entries: map[string]StateEntry{
					"m1//v1/ConfigMap/default/config": {ContentHash: "old123"},
				},
			},
			currentEntries: map[string]StateEntry{
				"m1//v1/ConfigMap/default/config": {ContentHash: "new456"},
			},
			alwaysSyncKeys: nil,
			expected: []DiffEntry{
				{Key: "m1//v1/ConfigMap/default/config", DiffType: DiffTypeModified, OldHash: "old123", NewHash: "new456"},
			},
		},
		{
			name: "removed: present in state but not current",
			stateData: &StateData{
				Entries: map[string]StateEntry{
					"m1//v1/ConfigMap/default/config": {ContentHash: "abc123"},
				},
			},
			currentEntries: map[string]StateEntry{},
			alwaysSyncKeys: nil,
			expected: []DiffEntry{
				{Key: "m1//v1/ConfigMap/default/config", DiffType: DiffTypeRemoved, OldHash: "abc123"},
			},
		},
		{
			name: "always-sync: GenesisSecret resource",
			stateData: &StateData{
				Entries: map[string]StateEntry{
					"m1//v1/Secret/default/my-secret": {ContentHash: "old123"},
				},
			},
			currentEntries: map[string]StateEntry{
				"m1//v1/Secret/default/my-secret": {ContentHash: "old123"},
			},
			alwaysSyncKeys: map[string]bool{
				"m1//v1/Secret/default/my-secret": true,
			},
			expected: []DiffEntry{
				{Key: "m1//v1/Secret/default/my-secret", DiffType: DiffTypeAlwaysSync, NewHash: "old123"},
			},
		},
		{
			name: "unchanged: identical hash with no changes",
			stateData: &StateData{
				Entries: map[string]StateEntry{
					"m1//v1/ConfigMap/default/config": {ContentHash: "same"},
				},
			},
			currentEntries: map[string]StateEntry{
				"m1//v1/ConfigMap/default/config": {ContentHash: "same"},
			},
			alwaysSyncKeys: nil,
			expected:       nil,
		},
		{
			name: "combined case: added + modified + removed + always-sync",
			stateData: &StateData{
				Entries: map[string]StateEntry{
					"m1//v1/ConfigMap/default/existing":  {ContentHash: "old"},
					"m1//v1/Deployment/default/removed":  {ContentHash: "rem"},
					"m1//v1/Secret/default/always":       {ContentHash: "sec"},
					"m1//v1/ConfigMap/default/unchanged": {ContentHash: "same"},
				},
			},
			currentEntries: map[string]StateEntry{
				"m1//v1/ConfigMap/default/existing":  {ContentHash: "new"},
				"m1//v1/Service/default/added":       {ContentHash: "add"},
				"m1//v1/Secret/default/always":       {ContentHash: "sec"},
				"m1//v1/ConfigMap/default/unchanged": {ContentHash: "same"},
			},
			alwaysSyncKeys: map[string]bool{
				"m1//v1/Secret/default/always": true,
			},
			expected: []DiffEntry{
				{Key: "m1//v1/Service/default/added", DiffType: DiffTypeAdded, NewHash: "add"},
				{Key: "m1//v1/ConfigMap/default/existing", DiffType: DiffTypeModified, OldHash: "old", NewHash: "new"},
				{Key: "m1//v1/Deployment/default/removed", DiffType: DiffTypeRemoved, OldHash: "rem"},
				{Key: "m1//v1/Secret/default/always", DiffType: DiffTypeAlwaysSync, NewHash: "sec"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeDiff(tt.stateData, tt.currentEntries, tt.alwaysSyncKeys)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d entries, got %d: %+v", len(tt.expected), len(result), result)
			}

			for i, expected := range tt.expected {
				got := result[i]
				if got.Key != expected.Key {
					t.Errorf("entry[%d] key: expected %q, got %q", i, expected.Key, got.Key)
				}
				if got.DiffType != expected.DiffType {
					t.Errorf("entry[%d] diffType: expected %q, got %q", i, expected.DiffType, got.DiffType)
				}
				if got.OldHash != expected.OldHash {
					t.Errorf("entry[%d] oldHash: expected %q, got %q", i, expected.OldHash, got.OldHash)
				}
				if got.NewHash != expected.NewHash {
					t.Errorf("entry[%d] newHash: expected %q, got %q", i, expected.NewHash, got.NewHash)
				}
			}
		})
	}
}
