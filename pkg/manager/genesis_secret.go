package manager

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"gopkg.in/yaml.v3"
	kyaml "sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenesisSecret managerはGenesisSecretのapplyに関する諸々をやります
type GenesisSecret struct {
	client   client.Client
	provider genesissecret.SecretProvider
}

func NewGenesisSecret(
	client client.Client,
	provider genesissecret.SecretProvider,
) *GenesisSecret {
	return &GenesisSecret{
		client:   client,
		provider: provider,
	}
}

// Apply implements Manager.
func (g *GenesisSecret) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	f, err := os.Open(m.Path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.WithStack(cerr)
		}
	}()

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.NewDecoder(f).Decode(&genesisSecret); err != nil {
		return errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := g.provider.Fetch(ctx, s)
		if err != nil {
			return errors.WithStack(err)
		}
		items = merge(items, i)
	}

	for _, o := range genesisSecret.Spec.Outputs {
		if o.KubernetesSecret == nil {
			return fmt.Errorf(".spec.output currently supports only KubernetesSecret")
		}

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

	return nil
}

// Destroy implements Manager.
func (g *GenesisSecret) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) (err error) {
	f, err := os.Open(m.Path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.WithStack(cerr)
		}
	}()

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.NewDecoder(f).Decode(&genesisSecret); err != nil {
		return errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := g.provider.Fetch(ctx, s)
		if err != nil {
			return errors.WithStack(err)
		}
		items = merge(items, i)
	}

	for _, o := range genesisSecret.Spec.Outputs {
		if o.KubernetesSecret == nil {
			return fmt.Errorf(".spec.output currently supports only KubernetesSecret")
		}

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

	return nil
}

var _ Manager = &GenesisSecret{}

// Build implements Manager.
func (g *GenesisSecret) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (s string, err error) {
	f, err := os.Open(m.Path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.WithStack(cerr)
		}
	}()

	genesisSecret := v1.GenesisSecret{}
	if err := yaml.NewDecoder(f).Decode(&genesisSecret); err != nil {
		return "", errors.WithStack(err)
	}

	items := map[string]string{}
	for _, s := range genesisSecret.Spec.Secrets {
		i, err := g.provider.Fetch(ctx, s)
		if err != nil {
			return "", errors.WithStack(err)
		}
		items = merge(items, i)
	}

	if len(genesisSecret.Spec.Outputs) == 0 {
		return "", fmt.Errorf("no outputs defined")
	}
	o := genesisSecret.Spec.Outputs[0]
	if o.KubernetesSecret == nil {
		return "", fmt.Errorf(".spec.output currently supports only KubernetesSecret")
	}

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

	// Kubernetes manifestとして正しいyamlを生成するために、
	// sigs.k8s.io/yaml を使う必要がある
	out, err := kyaml.Marshal(secret)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(out), nil
}

func merge(m1, m2 map[string]string) map[string]string {
	for k, v := range m2 {
		m1[k] = v
	}
	return m1
}
