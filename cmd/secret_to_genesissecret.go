package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// secretToGenesisSecretCmd represents the secret-to-genesissecret command
var secretToGenesisSecretCmd = &cobra.Command{
	Use:   "secret-to-genesissecret",
	Short: "Save existing cluster Secrets to 1Password and generate the corresponding GenesisSecret",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger, err := cliutil.NewLogger(cmd)
		if err != nil {
			return err
		}

		k8sClient, err := cliutil.NewK8sClient()
		if err != nil {
			return err
		}

		// フラグのパースは cmd 層で行い、options 構造体に詰めて runner へ渡す。
		// GetString 等のエラーを握り潰すとフィルタが黙って外れて namespace の
		// 全 Secret が対象になりうるため、必ずエラーを返す。
		opts, err := secretToGenesisSecretOptionsFromFlags(cmd)
		if err != nil {
			return err
		}

		opClient := &op.CommandClient{}
		r := runner.NewTazunaRunner(logger, k8sClient, opClient)
		if err := r.SecretToGenesisSecret(cmd.Context(), opts, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

// secretToGenesisSecretOptionsFromFlags は CLI フラグをパースして
// runner.SecretToGenesisSecretOptions を組み立てる。
func secretToGenesisSecretOptionsFromFlags(cmd *cobra.Command) (runner.SecretToGenesisSecretOptions, error) {
	opts := runner.SecretToGenesisSecretOptions{}

	var err error
	if opts.LabelSelector, err = cmd.Flags().GetString("label-selector"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.NameRegex, err = cmd.Flags().GetString("name-regex"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.Vault, err = cmd.Flags().GetString("vault"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.Namespace, err = cmd.Flags().GetString("namespace"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.DryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.DumpDir, err = cmd.Flags().GetString("dump-dir"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.Note, err = cmd.Flags().GetString("note"); err != nil {
		return opts, errors.WithStack(err)
	}
	if opts.OpHost, err = cmd.Flags().GetString("op-host"); err != nil {
		return opts, errors.WithStack(err)
	}

	return opts, nil
}

func init() {
	secretToGenesisSecretCmd.Flags().String("label-selector", "", "Target Secrets matching this label selector (key1=value1,key2=value2)")
	secretToGenesisSecretCmd.Flags().String("name-regex", "", "Target Secrets whose name matches this regex")
	secretToGenesisSecretCmd.Flags().String("vault", "", "1Password vault name")
	secretToGenesisSecretCmd.Flags().String("namespace", "default", "the Kubernetes namespace")
	secretToGenesisSecretCmd.Flags().Bool("dry-run", false, "dry run")
	secretToGenesisSecretCmd.Flags().String("dump-dir", ".", "the directory to dump the generated YAML files")
	secretToGenesisSecretCmd.Flags().String("note", "", "the note for the 1Password item")
	secretToGenesisSecretCmd.Flags().String("op-host", "", "Host part of the 1Password service account URL (e.g. example.1password.com)")
	_ = secretToGenesisSecretCmd.MarkFlagRequired("op-host")

	vaultCompletionFn := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		vault := op.NewVaultCommandBuilder().WithList(op.NewVaultListCommandBuilder())
		vaultListCommand := op.NewCommandBuilder().WithJSONFormat().WithVault(vault).Build()
		out, err := exec.Command(vaultListCommand[0], vaultListCommand[1:]...).Output()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		vaultList := []op.Vault{}
		if err := json.Unmarshal(out, &vaultList); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		vaultNames := make([]string, len(vaultList))
		for i, v := range vaultList {
			vaultNames[i] = v.Name
		}

		return vaultNames, cobra.ShellCompDirectiveNoFileComp
	}
	if err := secretToGenesisSecretCmd.RegisterFlagCompletionFunc("vault", vaultCompletionFn); err != nil {
		panic(err)
	}
	namespaceCompletionFn := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		kubeConfig, err := ctrl.GetConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		k8sClient, err := client.New(kubeConfig, client.Options{})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		namespaces := &corev1.NamespaceList{}
		if err := k8sClient.List(context.TODO(), namespaces); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		namespaceNames := make([]string, len(namespaces.Items))
		for i, ns := range namespaces.Items {
			namespaceNames[i] = ns.Name
		}
		return namespaceNames, cobra.ShellCompDirectiveNoFileComp
	}
	if err := secretToGenesisSecretCmd.RegisterFlagCompletionFunc("namespace", namespaceCompletionFn); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(secretToGenesisSecretCmd)
}
