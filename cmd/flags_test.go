package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// flagSpec captures the expected shape of a flag on a subcommand: presence,
// default value, and whether it is registered as a persistent flag on the
// nearest ancestor (when persistent is true, the flag is looked up via
// InheritedFlags after AddCommand wiring).
type flagSpec struct {
	name       string
	defaultVal string
	persistent bool
}

func assertFlag(t *testing.T, cmd *cobra.Command, spec flagSpec) {
	t.Helper()
	var f = cmd.Flags().Lookup(spec.name)
	if f == nil && spec.persistent {
		f = cmd.InheritedFlags().Lookup(spec.name)
	}
	if f == nil {
		t.Fatalf("flag %q not registered on %s", spec.name, cmd.Use)
	}
	if f.DefValue != spec.defaultVal {
		t.Errorf("flag %q on %s: default = %q, want %q", spec.name, cmd.Use, f.DefValue, spec.defaultVal)
	}
}

func TestRootPersistentFlags(t *testing.T) {
	t.Parallel()
	for _, spec := range []flagSpec{
		{name: "file-path", defaultVal: "tazuna.yaml"},
		{name: "log-level", defaultVal: "info"},
	} {
		if f := rootCmd.PersistentFlags().Lookup(spec.name); f == nil {
			t.Errorf("root persistent flag %q missing", spec.name)
		} else if f.DefValue != spec.defaultVal {
			t.Errorf("root persistent flag %q default = %q, want %q", spec.name, f.DefValue, spec.defaultVal)
		}
	}
}

func TestSubcommandFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cmd   *cobra.Command
		flags []flagSpec
	}{
		{
			cmd: applyCmd,
			flags: []flagSpec{
				{name: "tags", defaultVal: "[]"},
				{name: "no-cache", defaultVal: "false"},
				{name: "offline", defaultVal: "false"},
				{name: "sync", defaultVal: "false"},
				{name: "prune", defaultVal: "false"},
				{name: "atomic", defaultVal: "false"},
			},
		},
		{
			cmd: destroyCmd,
			flags: []flagSpec{
				{name: "force", defaultVal: "false"},
				{name: "tags", defaultVal: "[]"},
				{name: "no-cache", defaultVal: "false"},
				{name: "offline", defaultVal: "false"},
			},
		},
		{
			cmd: buildCmd,
			flags: []flagSpec{
				{name: "tags", defaultVal: "[]"},
				{name: "no-cache", defaultVal: "false"},
				{name: "offline", defaultVal: "false"},
			},
		},
		{
			cmd: planCmd,
			flags: []flagSpec{
				{name: "tags", defaultVal: "[]"},
			},
		},
		{
			cmd: checkCmd,
			flags: []flagSpec{
				{name: "fix", defaultVal: "false"},
			},
		},
		{
			cmd: tagsCmd,
			flags: []flagSpec{
				{name: "tags", defaultVal: "[]"},
			},
		},
		{
			cmd: secretToGenesisSecretCmd,
			flags: []flagSpec{
				{name: "label-selector", defaultVal: ""},
				{name: "name-regex", defaultVal: ""},
				{name: "vault", defaultVal: ""},
				{name: "namespace", defaultVal: "default"},
				{name: "dry-run", defaultVal: "false"},
				{name: "dump-dir", defaultVal: "."},
				{name: "note", defaultVal: ""},
				{name: "op-host", defaultVal: ""},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.cmd.Use, func(t *testing.T) {
			t.Parallel()
			for _, spec := range tc.flags {
				assertFlag(t, tc.cmd, spec)
			}
		})
	}
}

func TestSecretToGenesisSecretRequiresOpHost(t *testing.T) {
	t.Parallel()
	annotations := secretToGenesisSecretCmd.Flag("op-host").Annotations
	required, ok := annotations[cobra.BashCompOneRequiredFlag]
	if !ok || len(required) == 0 || required[0] != "true" {
		t.Errorf("op-host flag is not marked required: annotations=%v", annotations)
	}
}

func TestSubcommandsAreRegisteredOnRoot(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"apply":                   true,
		"destroy":                 true,
		"build":                   true,
		"check":                   true,
		"plan":                    true,
		"status":                  true,
		"tags":                    true,
		"state":                   true,
		"secret-to-genesissecret": true,
		"version":                 true,
	}
	for _, c := range rootCmd.Commands() {
		delete(want, c.Name())
	}
	if len(want) != 0 {
		t.Errorf("subcommands missing from root: %v", want)
	}
}

func TestStateSubcommandsAreRegistered(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"list":  true,
		"diff":  true,
		"drift": true,
	}
	for _, c := range stateCmd.Commands() {
		delete(want, c.Name())
	}
	if len(want) != 0 {
		t.Errorf("state subcommands missing: %v", want)
	}
}

func TestStateSyncSubcommandIsRemoved(t *testing.T) {
	t.Parallel()
	for _, c := range stateCmd.Commands() {
		if c.Name() == "sync" {
			t.Errorf("state sync subcommand should not exist after merge into apply")
		}
	}
}
