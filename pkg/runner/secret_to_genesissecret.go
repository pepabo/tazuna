package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// SecretToGenesisSecret は、KubernetesのSecretを1Passwordに保存し、
// 対応するGenesisSecretのYAMLを生成する
func (t *TazunaRunner) SecretToGenesisSecret(
	ctx context.Context,
	cmd *cobra.Command,
	opClient op.Client,
	k8sClient client.Client,
	opHost string,
) error {
	// STEP1: KubernetesのsecretListを取得する
	// STEP2: 取得したsecretListを元に、1PasswordのItemCreateCommandsを実行する
	// STEP3: 作成した1PasswordのItemに対応する、GenesisSecretのYAMLを生成する

	secretList := corev1.SecretList{}
	listOptions, err := secretToGenesisSecretCommandToSecretListOptions(cmd)
	if err != nil {
		return err
	}

	// STEP1
	t.logger.InfoContext(ctx, "list secrets")
	if err := k8sClient.List(ctx, &secretList, listOptions...); err != nil {
		return errors.WithStack(err)
	}
	t.logger.DebugContext(ctx, "successfully listed secrets", slog.Int("count", len(secretList.Items)))

	nameRegex, err := cmd.Flags().GetString("name-regex")
	if err != nil {
		return errors.WithStack(err)
	}
	var nameRegexp *regexp.Regexp
	if nameRegex != "" {
		nameRegexp, err = regexp.Compile(nameRegex)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	secrets := filterSecretsWithRegex(secretList.Items, nameRegexp)
	t.logger.DebugContext(ctx, "filtered by name-regex", slog.Int("count", len(secrets)))

	vaultName, err := cmd.Flags().GetString("vault")
	if err != nil {
		return errors.WithStack(err)
	}

	// STEP2
	t.logger.InfoContext(ctx, "get the vault that the vault items stores", slog.String("vault", vaultName))
	vault, err := opClient.GetVault(ctx, vaultName)
	if err != nil {
		return errors.WithStack(err)
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return errors.WithStack(err)
	}

	note, err := cmd.Flags().GetString("note")
	if err != nil {
		return errors.WithStack(err)
	}
	items := secretsToVaultItems(secrets, &vault, note)

	commands, err := itemCreateCommandsFromItems(items, vaultName, dryRun)
	if err != nil {
		return errors.WithStack(err)
	}

	existItems, err := opClient.ListVaultItems(ctx, vaultName)
	if err != nil {
		return errors.WithStack(err)
	}

	for i, command := range commands {
		item := items[i]

		found := false
		for _, existItem := range existItems {
			if existItem.Title == item.Title {
				found = true
				break
			}
		}
		if found {
			t.logger.InfoContext(ctx, "item already exists in vault", slog.String("item", item.Title), slog.String("vault", vaultName))
			continue
		}

		t.logger.InfoContext(ctx, "execute command", slog.String("command", command.String()))
		if err := command.Run(); err != nil {
			return errors.WithStack(err)
		}
	}

	// STEP3
	dumpDir, err := cmd.Flags().GetString("dump-dir")
	if err != nil {
		return errors.WithStack(err)
	}

	for i := range items {
		secret := secrets[i]
		item := items[i]

		gsItems := map[string]v1.GenesisSecretGenerateItem{}

		keys := []string{}
		for k := range secret.Data {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, key := range keys {
			gsItems[key] = v1.GenesisSecretGenerateItem{
				MapTo: key,
			}
		}

		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		labels := secret.Labels
		labels["tazuna.pepabo.com/managed-by"] = "tazuna"
		gs := v1.GenesisSecret{
			Spec: v1.GenesisSecretSpec{
				Secrets: []v1.GenesisSecretGenerate{
					{
						PreferLabel: true, // tls.key/tls.crtを使うために必要
						URI:         fmt.Sprintf("op://%s/%s/%s", opHost, vaultName, item.Title),
						Items:       gsItems,
					},
				},
				Outputs: []v1.GenesisSecretOutput{
					{
						KubernetesSecret: &v1.GenesisSecretOutputKubernetesSecret{
							Name:      secret.Name,
							Namespace: secret.Namespace,
							Labels:    labels,
							Type:      string(secret.Type),
						},
					},
				},
			},
		}
		path := filepath.Join(dumpDir, fmt.Sprintf("%s.yaml", secret.Name))

		t.logger.DebugContext(ctx, "generate GenesisSecret", slog.String("path", path))

		out, err := yaml.Marshal(gs)
		if err != nil {
			return errors.WithStack(err)
		}

		if dryRun {
			fmt.Printf("DRYRUN: generate GenesisSecret into %s\n", path)
			continue
		}

		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err := f.Write(out); err != nil {
			return errors.WithStack(err)
		}

		if err := f.Close(); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// secretToGenesisSecretCommandToSecretListOptions は、
// cobra.CommandからKubernetesのSecretListOptionsを生成する
func secretToGenesisSecretCommandToSecretListOptions(cmd *cobra.Command) ([]client.ListOption, error) {
	listOptions := []client.ListOption{}
	if labelSelectorStr, err := cmd.Flags().GetString("label-selector"); err == nil && labelSelectorStr != "" {
		labelSelector, err := labelSelectorStringToMap(labelSelectorStr)
		if err != nil {
			return listOptions, errors.WithStack(err)
		}
		listOptions = append(listOptions, client.MatchingLabels(labelSelector))
	}

	if ns, err := cmd.Flags().GetString("namespace"); err == nil && ns != "" {
		listOptions = append(listOptions, client.InNamespace(ns))
	}

	return listOptions, nil
}

func filterSecretsWithRegex(
	secrets []corev1.Secret,
	nameRegexp *regexp.Regexp,
) []corev1.Secret {
	results := []corev1.Secret{}
	for _, secret := range secrets {
		if nameRegexp != nil && !nameRegexp.MatchString(secret.Name) {
			continue
		}

		results = append(results, secret)
	}

	return results
}

func secretToVaultItem(secret corev1.Secret) op.Item {
	fields := []op.ItemField{}

	for k, v := range secret.Data {
		field := op.ItemField{
			Label: k,
			Type:  op.ItemFieldTypeString,
			Value: string(v),
		}
		fields = append(fields, field)
	}
	return op.Item{
		Title:  secret.Name,
		Fields: fields,
	}
}

func labelSelectorStringToMap(labelSelectorStr string) (map[string]string, error) {
	labelSelector := make(map[string]string)
	labelSelectorStrs := strings.Split(labelSelectorStr, ",")
	for _, label := range labelSelectorStrs {
		kv := strings.Split(label, "=")
		if len(kv) != 2 {
			return nil, errors.New("label selector must be key=value")
		}
		labelSelector[kv[0]] = kv[1]
	}
	return labelSelector, nil
}

func secretsToVaultItems(
	secrets []corev1.Secret,
	vault *op.Vault,
	note string,
) []op.Item {
	items := []op.Item{}
	for _, secret := range secrets {
		item := secretToVaultItem(secret)
		item.Vault = *vault
		// メモがないとなんのitemなのかなんもわからないのでいれる
		item.Fields = append(item.Fields, op.ItemField{
			ID:      op.ItemIDNotesPlain,
			Type:    op.ItemFieldTypeString,
			Purpose: op.ItemPurposeNotes,
			Label:   op.ItemLabelNotesPlain,
			Value: fmt.Sprintf(`This item was automatically generated by Tazuna.
%s`, note),
		})
		items = append(items, item)
	}
	return items
}

func itemCreateCommandsFromItems(
	items []op.Item,
	vaultName string,
	dryRun bool,
) ([]*exec.Cmd, error) {
	if err := op.ValidateIdentifier("vault", vaultName); err != nil {
		return nil, errors.WithStack(err)
	}
	commands := []*exec.Cmd{}
	for _, item := range items {
		if err := op.ValidateIdentifier("item", item.Title); err != nil {
			return nil, errors.WithStack(err)
		}
		category := op.VaultItemCategoryAPICredential
		itemCreateCommand := op.NewCommandBuilder().
			WithItem(
				op.NewItemCommandBuilder().
					WithCreate(
						op.NewItemCreateCommandBuilder().
							WithTitle(item.Title).
							WithCategory(&category).
							WithVault(vaultName).
							WithStdin(true).
							WithDryRun(dryRun),
					),
			).Build()

		cmd := exec.Command(itemCreateCommand[0], itemCreateCommand[1:]...)
		itemBytes, err := json.Marshal(item)
		if err != nil {
			return commands, errors.WithStack(err)
		}

		cmd.Stdin = bytes.NewBuffer(itemBytes)
		commands = append(commands, cmd)
	}

	return commands, nil
}
