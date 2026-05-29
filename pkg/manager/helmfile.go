package manager

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/hint"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/resource"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cockroachdb/errors"
	"github.com/helmfile/helmfile/pkg/app"
	"github.com/helmfile/helmfile/pkg/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Helmfile struct {
	client   client.Client
	opClient op.Client
}

// Destroy implements Manager.
func (h *Helmfile) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	globalImpl, err := h.setupGlobalImpl(ctx, &m)
	if err != nil {
		return errors.WithStack(err)
	}
	templateImpl, err := h.setupTemplateImpl(ctx, &m, globalImpl)
	if err != nil {
		return errors.WithStack(err)
	}

	a := app.New(templateImpl)

	// Loggerを入れないとsegvで落ちるのでzap loggerを初期化する
	a.Logger = zap.NewNop().Sugar()

	// helmfileパッケージではtemplateの書き出しを設定することはできず、
	// https://github.com/helmfile/helmfile/blob/b5eb879357d0eae3ad914a38f7221bed94573cb6/pkg/testutil/testutil.go という関数を用いて標準出力をキャプチャしています。
	// 提供されるCaptureOutputではクロージャがerrorを返すことができないため、forkした関数を作ります。
	out, err := captureStdout(func() error {
		if err := a.Template(templateImpl); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return errors.WithStack(err)
	}

	objects, err := manifest.ConvertManifestsToObjects([]byte(out), m.Helmfile.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to delete an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.DeleteObject(ctx, h.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Apply implements Manager.
func (h *Helmfile) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	// NOTE: helmfileのapp.Sync() でcontroller-runtimeのfake clientを差し込むことはできないかつ、
	//       対象のクラスタにhelmの管理情報を保存しないtまえ、
	//       helmfile templateで生成したKubernetesマニフェスト群をclientでapplyする方針を利用します。
	//       これによりhelmのrollbackを利用できなくなりますが、クラスタのbootstrapという観点ではrollbackの機能はいらないものとします。
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	globalImpl, err := h.setupGlobalImpl(ctx, &m)
	if err != nil {
		return errors.WithStack(err)
	}
	templateImpl, err := h.setupTemplateImpl(ctx, &m, globalImpl)
	if err != nil {
		return errors.WithStack(err)
	}
	a := app.New(templateImpl)

	// Loggerを入れないとsegvで落ちるのでzap loggerを初期化する
	a.Logger = zap.NewNop().Sugar()

	// helmfileパッケージではtemplateの書き出しを設定することはできず、
	// https://github.com/helmfile/helmfile/blob/b5eb879357d0eae3ad914a38f7221bed94573cb6/pkg/testutil/testutil.go という関数を用いて標準出力をキャプチャしています。
	// 提供されるCaptureOutputではクロージャがerrorを返すことができないため、forkした関数を作ります。
	out, err := captureStdout(func() error {
		if err := a.Template(templateImpl); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return errors.WithStack(err)
	}

	objects, err := manifest.ConvertManifestsToObjects([]byte(out), m.Helmfile.DefaultNamespace)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, obj := range objects {
		logger.DebugContext(ctx, "trying to create or update an object", slog.String("namespace", obj.GetNamespace()), slog.String("name", obj.GetName()), slog.String("kind", obj.GetObjectKind().GroupVersionKind().Kind))
		if err := resource.CreateOrUpdateForObject(ctx, h.client, obj); err != nil {
			return errors.WithStack(err)
		}
	}

	// Wait が設定されている場合は、リソースが Ready になるまで待つ
	if m.Helmfile.Wait {
		if err := h.waitForResources(ctx, logger, objects, m.Helmfile.TimeoutSeconds); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// waitForResources は、指定されたリソースが Ready になるまで待機します
func (h *Helmfile) waitForResources(ctx context.Context, logger *slog.Logger, objects []client.Object, timeout int) error {
	// デフォルトのタイムアウトは 5 分
	if timeout == 0 {
		timeout = 300
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	for _, obj := range objects {
		if err := h.waitForResource(timeoutCtx, logger, obj); err != nil {
			return errors.Wrapf(err, "failed to wait for resource %s/%s", obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

// waitForResource は、単一のリソースが Ready になるまで待機します
func (h *Helmfile) waitForResource(ctx context.Context, logger *slog.Logger, obj client.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	logger.InfoContext(ctx, "waiting for resource to be ready",
		slog.String("namespace", obj.GetNamespace()),
		slog.String("name", obj.GetName()),
		slog.String("kind", gvk.Kind))

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("timeout waiting for %s %s/%s to be ready", gvk.Kind, obj.GetNamespace(), obj.GetName())
		case <-ticker.C:
			ready, err := h.isResourceReady(ctx, obj)
			if err != nil {
				return errors.WithStack(err)
			}
			if ready {
				logger.InfoContext(ctx, "resource is ready",
					slog.String("namespace", obj.GetNamespace()),
					slog.String("name", obj.GetName()),
					slog.String("kind", gvk.Kind))
				return nil
			}
		}
	}
}

func (h *Helmfile) setupGlobalImpl(ctx context.Context, m *v1.Manifest) (*config.GlobalImpl, error) {
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	globalConfig := new(config.GlobalOptions)
	globalConfig.File = m.Path

	globalImpl := config.NewGlobalImpl(globalConfig)
	return globalImpl, nil
}

func (h *Helmfile) setupTemplateImpl(ctx context.Context, m *v1.Manifest, globalImpl *config.GlobalImpl) (*config.TemplateImpl, error) {
	templateOptions := config.NewTemplateOptions()
	templateOptions.IncludeCRDs = m.Helmfile.IncludeCRDs
	if m.Helmfile.KubeVersion != "" {
		templateOptions.KubeVersion = m.Helmfile.KubeVersion
	}

	// extraValueFilesを追加
	if len(m.Helmfile.ExtraValueFiles) > 0 {
		templateOptions.Values = append(templateOptions.Values, m.Helmfile.ExtraValueFiles...)
	}

	templateImpl := config.NewTemplateImpl(globalImpl, templateOptions)
	varSet, err := h.ConstructHelmfileVars(ctx, m)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct helmfile vars for manifest %s", m.Path)
	}
	templateImpl.SetSet(varSet)
	return templateImpl, nil
}

// isResourceReady は、リソースが Ready 状態かどうかを確認します。
// 判定ロジック自体は pkg/resource に切り出されており、本メソッドはライブ取得した
// unstructured を resource.IsReady に委譲する薄いラッパーです。
func (h *Helmfile) isResourceReady(ctx context.Context, obj client.Object) (bool, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	key := client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	// リソースの最新状態を unstructured で取得
	// manifest.ConvertManifestsToObjects が *unstructured.Unstructured を返すため、
	// client.Get の結果も unstructured で受け取る
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	if err := h.client.Get(ctx, key, current); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// リソースがまだ存在しない場合は ready ではない
			return false, nil
		}
		return false, errors.WithStack(err)
	}

	return resource.IsReady(current)
}

func (h *Helmfile) ConstructHelmfileVars(ctx context.Context, m *v1.Manifest) (map[string]any, error) {
	vars := make(map[string]any)

	for k, v := range m.Helmfile.Vars {
		if v.From == "" {
			return nil, errors.Errorf("helmfile var %s has no From field, supported From is 'env/static/op'", k)
		}

		switch v.From {
		case v1.HelmFileVarFromEnv:
			if v.Env == nil {
				return nil, errors.Errorf("helmfile var %s has From env but no env field", k)
			}
			env, ok := os.LookupEnv(*v.Env)
			if !ok {
				return nil, errors.Errorf("helmfile var %s has From env but environment variable %s is not set", k, *v.Env)
			}
			vars[k] = env
		case v1.HelmFileVarFromStatic:
			// 実行時の安全性のためのvalidation（通常は ValidateTazuna() で防がれる）
			count := 0
			if v.Static != nil {
				count++
			}
			if v.StaticSlice != nil {
				count++
			}
			if v.StaticMap != nil {
				count++
			}
			if count == 0 {
				return nil, errors.Errorf("helmfile var %s has From static but no static/staticSlice/staticMap field", k)
			}
			if count > 1 {
				return nil, errors.Errorf("helmfile var %s has From static but multiple static fields are set", k)
			}

			if v.Static != nil {
				vars[k] = *v.Static
			} else if v.StaticSlice != nil {
				vars[k] = v.StaticSlice
			} else if v.StaticMap != nil {
				vars[k] = v.StaticMap
			}
		case v1.HelmFileVarFromOp:
			if h.opClient == nil {
				return nil, errors.New("helmfile var has From op but OnePassword client is not set")
			}
			if v.Op == nil {
				return nil, errors.Errorf("helmfile var %s has From op but no op field", k)
			}

			if v.Op.Key == "" || v.Op.Vault == "" || v.Op.Item == "" || v.Op.Field == "" {
				return nil, errors.Errorf("helmfile var %s has From op but op field is not set properly", k)
			}

			item, err := h.opClient.GetVaultItem(ctx, v.Op.Vault, v.Op.Item)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get vault item %s from %s", v.Op.Item, v.Op.Vault)
			}

			for _, field := range item.Fields {
				if v.Op.Key == v1.HelmFileVarOpKeyID {
					if field.ID == v.Op.Field {
						vars[k] = field.Value
						break
					}
				} else if v.Op.Key == v1.HelmFileVarOpKeyLabel {
					if field.Label == v.Op.Field {
						vars[k] = field.Value
						break
					}
				}
			}

			if vars[k] == "" {
				return nil, errors.Errorf("helmfile var %s has From op but op field %s not found in item %s", k, v.Op.Field, v.Op.Item)
			}
		}
	}

	// hint処理: tazuna.hint.yamlが存在すれば検証・デフォルト注入を行う
	hintDir := filepath.Dir(m.Path)
	hintFile, err := hint.LoadHintFile(hintDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load hint file from %s", hintDir)
	}
	if hintFile != nil {
		if err := hint.ValidateHint(hintFile); err != nil {
			return nil, errors.Wrap(err, "invalid hint file")
		}
		if err := hint.ValidateVarsAgainstHint(hintFile, m.Helmfile.Vars); err != nil {
			return nil, errors.Wrap(err, "vars validation against hint failed")
		}
		vars, err = hint.MergeVarsWithHint(hintFile, vars)
		if err != nil {
			return nil, errors.Wrap(err, "failed to merge vars with hint")
		}
	}

	return vars, nil
}

var _ Manager = &Helmfile{}

func NewHelmfile(client client.Client, opClient op.Client) *Helmfile {
	return &Helmfile{client, opClient}
}

// ref: https://github.com/helmfile/helmfile/blob/b5eb879357d0eae3ad914a38f7221bed94573cb6/pkg/testutil/testutil.go
func captureStdout(f func() error) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	stdout := os.Stdout
	defer func() {
		os.Stdout = stdout
	}()
	os.Stdout = writer
	out := make(chan string, 1)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	var ioCopyErr error
	go func() {
		var buf bytes.Buffer
		defer wg.Done()
		_, ioCopyErr = io.Copy(&buf, reader)
		out <- buf.String()
	}()
	if err := f(); err != nil {
		return "", err
	}
	_ = writer.Close()
	wg.Wait()
	if ioCopyErr != nil {
		return "", ioCopyErr
	}
	return <-out, nil
}

// Build implements Manager.
func (h *Helmfile) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error) {
	if m.Helmfile == nil {
		m.Helmfile = v1.DefaultHelmfile()
	}

	globalImpl, err := h.setupGlobalImpl(ctx, &m)
	if err != nil {
		return "", errors.WithStack(err)
	}
	templateImpl, err := h.setupTemplateImpl(ctx, &m, globalImpl)
	if err != nil {
		return "", errors.WithStack(err)
	}
	a := app.New(templateImpl)

	// Loggerを入れないとsegvで落ちるのでzap loggerを初期化する
	a.Logger = zap.NewNop().Sugar()

	// helmfileパッケージではtemplateの書き出しを設定することはできず、
	// https://github.com/helmfile/helmfile/blob/b5eb879357d0eae3ad914a38f7221bed94573cb6/pkg/testutil/testutil.go という関数を用いて標準出力をキャプチャしています。
	// 提供されるCaptureOutputではクロージャがerrorを返すことができないため、forkした関数を作ります。
	out, err := captureStdout(func() error {
		if err := a.Template(templateImpl); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	if err != nil {
		return "", errors.WithStack(err)
	}

	return out, nil
}
