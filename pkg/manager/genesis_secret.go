package manager

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenesisSecret managerはGenesisSecretのapplyに関する諸々をやります。
// SecretProvider の解決は registry 経由で manifest の .spec.provider を
// name として動的に lookup します。
type GenesisSecret struct {
	client   client.Client
	registry *genesissecret.ProviderRegistry
	// stdout は GenesisSecretOutputStdout の書き出し先。
	// 未設定なら os.Stdout を使う。テストで差し替えるためのフィールド。
	stdout io.Writer
}

// NewGenesisSecret は GenesisSecret manager を生成します。
// registry には少なくとも組み込みの "default-op" provider が登録されている想定です。
// GenesisSecret の .spec.provider が空文字の manifest は "default-op" にフォールバック
// されることで後方互換性が保たれます。
func NewGenesisSecret(
	c client.Client,
	registry *genesissecret.ProviderRegistry,
) *GenesisSecret {
	return &GenesisSecret{
		client:   c,
		registry: registry,
		stdout:   os.Stdout,
	}
}

// WithStdout は stdout 出力先を上書きします。テスト用。
func (g *GenesisSecret) WithStdout(w io.Writer) *GenesisSecret {
	g.stdout = w
	return g
}

// classifyOutput は GenesisSecretOutput がどの出力種別を要求しているかを判別します。
// 両方nil or 両方非nil はエラーとして扱います。
func classifyOutput(o v1.GenesisSecretOutput) (string, error) {
	hasStdout := o.Stdout != nil
	hasK8s := o.KubernetesSecret != nil
	switch {
	case hasStdout && hasK8s:
		return "", fmt.Errorf("output cannot specify both stdout and kubernetesSecret")
	case hasStdout:
		return "stdout", nil
	case hasK8s:
		return "kubernetesSecret", nil
	default:
		return "", fmt.Errorf("output must specify either stdout or kubernetesSecret")
	}
}

// writeItemsToStdout は items を sorted KEY=VALUE 形式で stdout writer に書き出します。
func (g *GenesisSecret) writeItemsToStdout(items map[string]string) error {
	w := g.stdout
	if w == nil {
		w = os.Stdout
	}
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s=%s\n", k, items[k]); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// resolveProvider は GenesisSecret manifest の .spec.provider 値から
// SecretProvider を取得します。空文字の場合は組み込みの "default-op" に
// フォールバックすることで、既存 fixture との後方互換を維持します。
func (g *GenesisSecret) resolveProvider(name string) (genesissecret.SecretProvider, error) {
	if g.registry == nil {
		return nil, fmt.Errorf("provider registry is not initialized")
	}
	if name == "" {
		name = v1.DefaultOnePasswordProviderName
	}
	return g.registry.Get(name)
}

// Apply implements Manager.
func (g *GenesisSecret) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	data, err := os.ReadFile(m.Path)
	if err != nil {
		return errors.WithStack(err)
	}

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.Unmarshal(data, &genesisSecret); err != nil {
		return errors.WithStack(err)
	}

	provider, err := g.resolveProvider(genesisSecret.Spec.Provider)
	if err != nil {
		return errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := provider.Fetch(ctx, s)
		if err != nil {
			return errors.WithStack(err)
		}
		items = merge(items, i)
	}

	for _, o := range genesisSecret.Spec.Outputs {
		kind, err := classifyOutput(o)
		if err != nil {
			return errors.WithStack(err)
		}

		switch kind {
		case "stdout":
			if err := g.writeItemsToStdout(items); err != nil {
				return errors.WithStack(err)
			}
		case "kubernetesSecret":
			secret := corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: o.KubernetesSecret.Namespace,
					Name:      o.KubernetesSecret.Name,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, g.client, &secret, func() error {
				secret.SetLabels(o.KubernetesSecret.Labels)
				secret.SetAnnotations(o.KubernetesSecret.Annotations)
				secret.StringData = items
				if o.KubernetesSecret.Type == "" {
					secret.Type = corev1.SecretTypeOpaque
				} else {
					secret.Type = corev1.SecretType(o.KubernetesSecret.Type)
				}
				return nil
			})
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return nil
}

// Destroy implements Manager.
func (g *GenesisSecret) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	data, err := os.ReadFile(m.Path)
	if err != nil {
		return errors.WithStack(err)
	}

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.Unmarshal(data, &genesisSecret); err != nil {
		return errors.WithStack(err)
	}

	provider, err := g.resolveProvider(genesisSecret.Spec.Provider)
	if err != nil {
		return errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := provider.Fetch(ctx, s)
		if err != nil {
			return errors.WithStack(err)
		}
		items = merge(items, i)
	}

	for _, o := range genesisSecret.Spec.Outputs {
		kind, err := classifyOutput(o)
		if err != nil {
			return errors.WithStack(err)
		}

		switch kind {
		case "stdout":
			// stdout 出力にはクラスタリソースが対応しないため Destroy では何もしない
			continue
		case "kubernetesSecret":
			secret := corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: o.KubernetesSecret.Namespace,
					Name:      o.KubernetesSecret.Name,
				},
			}

			if err := g.client.Delete(ctx, &secret); err != nil {
				if client.IgnoreNotFound(err) != nil {
					return errors.WithStack(err)
				}
			}
		}
	}

	return nil
}

var _ Manager = &GenesisSecret{}

// Build implements Manager.
func (g *GenesisSecret) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error) {
	data, err := os.ReadFile(m.Path)
	if err != nil {
		return "", errors.WithStack(err)
	}

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.Unmarshal(data, &genesisSecret); err != nil {
		return "", errors.WithStack(err)
	}

	provider, err := g.resolveProvider(genesisSecret.Spec.Provider)
	if err != nil {
		return "", errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := provider.Fetch(ctx, s)
		if err != nil {
			return "", errors.WithStack(err)
		}
		items = merge(items, i)
	}

	if len(genesisSecret.Spec.Outputs) == 0 {
		return "", fmt.Errorf("no outputs defined")
	}
	o := genesisSecret.Spec.Outputs[0]
	kind, err := classifyOutput(o)
	if err != nil {
		return "", errors.WithStack(err)
	}

	switch kind {
	case "stdout":
		// stdout 出力にはクラスタリソースが対応しないため Build は空文字を返す。
		// これにより state save 経路でも 0 entries となり state が書き込まれない。
		return "", nil
	case "kubernetesSecret":
		secret := corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: o.KubernetesSecret.Namespace,
				Name:      o.KubernetesSecret.Name,
			},
		}
		secret.SetLabels(o.KubernetesSecret.Labels)
		secret.SetAnnotations(o.KubernetesSecret.Annotations)
		secret.StringData = items
		if o.KubernetesSecret.Type == "" {
			secret.Type = corev1.SecretTypeOpaque
		} else {
			secret.Type = corev1.SecretType(o.KubernetesSecret.Type)
		}

		out, err := yaml.Marshal(secret)
		if err != nil {
			return "", errors.WithStack(err)
		}

		return string(out), nil
	}
	return "", fmt.Errorf("unsupported output kind: %s", kind)
}

func merge(m1, m2 map[string]string) map[string]string {
	for k, v := range m2 {
		m1[k] = v
	}
	return m1
}
