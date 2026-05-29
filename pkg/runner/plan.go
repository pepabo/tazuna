package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/manifest"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utildiff "k8s.io/apimachinery/pkg/util/diff"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// planChangeKind は plan で検出されるリソース変更の種類を表す
type planChangeKind string

const (
	// planChangeCreate は live にまだ存在しないリソース (新規作成予定) を表す
	planChangeCreate planChangeKind = "create"
	// planChangeUpdate は live と desired の間にフィールド差分があるリソースを表す
	planChangeUpdate planChangeKind = "update"
)

// planChange は 1 リソースに対する plan の検出結果を表す
type planChange struct {
	kind        planChangeKind
	resourceKey string // "Kind/namespace/name" 形式の表示用キー
	// diff は planChangeUpdate のときに live と desired を比較した unified diff 文字列。
	// planChangeCreate のときは空。
	diff string
}

// Plan は server-side dry-run のスローガン下で、tazuna.yaml で宣言された manifest を
// Build() でレンダリングし、ライブクラスタとフィールド単位で比較した結果を出力する。
//
// 実装は client-side diff である:
//   - fake client が server-side apply (client.Patch + client.Apply + client.DryRunAll)
//     を完全にはサポートしておらず、integration test が成立しないため
//   - util/diff.Diff を使った unified diff で「どのフィールドが変わるか」を可視化する
//
// 後ろ向きの正確性 (admission webhook や defaulting の結果は反映されない) を犠牲に、
// integration test 可能なロジックを優先している。
func (t *TazunaRunner) Plan(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
	w io.Writer,
) error {
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)
	t.providersBaseDir = baseDir

	managers, err := t.resolvePlanManagers(tazuna)
	if err != nil {
		return errors.Wrap(err, "failed to setup managers")
	}

	hasAnyChange := false
	for i, m := range tazuna.Spec.Manifests {
		if m.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping plan", slog.Int("index", i), slog.String("type", string(m.Type)))
			continue
		}

		// parallel manager は Build() 未対応のためスキップ
		if m.Type == v1.ManifestTypeParallel {
			t.logger.WarnContext(ctx, "parallel manifest is not supported for plan, skipping", slog.String("name", m.Name))
			continue
		}

		// GenesisSecret は always-sync 扱いで apply 時に毎回再シンクされるため、
		// plan のフィールド diff という概念に合わない。drift と同様にスキップする。
		if m.Type == v1.ManifestTypeGenesisSecret {
			t.logger.DebugContext(ctx, "genesis-secret manifest is always-sync, skipping plan", slog.String("name", m.Name))
			continue
		}

		mgr, ok := managers[string(m.Type)]
		if !ok {
			return fmt.Errorf("manager %s not found", m.Type)
		}

		out, err := mgr.Build(ctx, t.logger, m)
		if err != nil {
			return errors.Wrapf(err, "failed to build manifest %q", m.Name)
		}

		defaultNs := getDefaultNamespace(m)
		objects, err := manifest.ConvertManifestsToObjects([]byte(out), defaultNs)
		if err != nil {
			return errors.Wrapf(err, "failed to convert manifests to objects for %q", m.Name)
		}

		changes, err := t.computePlanChanges(ctx, objects)
		if err != nil {
			return errors.Wrapf(err, "failed to compute plan changes for %q", m.Name)
		}
		if len(changes) == 0 {
			continue
		}

		hasAnyChange = true
		if err := writePlanManifestHeader(w, m.Name); err != nil {
			return err
		}
		for _, ch := range changes {
			if err := writePlanChange(w, ch); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return errors.WithStack(err)
		}
	}

	if !hasAnyChange {
		if _, err := fmt.Fprintln(w, "No changes detected."); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// resolvePlanManagers は plan 用に manifest type -> Manager のマップを取得する。
// テスト用 override がセットされていればそちらを優先する。
func (t *TazunaRunner) resolvePlanManagers(tazuna v1.Tazuna) (map[string]manager.Manager, error) {
	if t.managersOverride != nil {
		out := make(map[string]manager.Manager, len(t.managersOverride))
		for k, v := range t.managersOverride {
			out[k] = v
		}
		return out, nil
	}
	return setupManagers(t.k8sClient, t.opClient, t.orasPullOpts, tazuna.Spec.Providers, t.providersBaseDir)
}

// computePlanChanges は desired (Build() 由来) オブジェクト群とライブクラスタの状態を
// 比較し、planChange のスライスとして返す。差分なしのものは含めない。
// 安定した出力のため resourceKey でソートする。
func (t *TazunaRunner) computePlanChanges(
	ctx context.Context,
	desiredObjects []client.Object,
) ([]planChange, error) {
	changes := make([]planChange, 0, len(desiredObjects))

	for _, desired := range desiredObjects {
		desiredUns, ok := desired.(*unstructured.Unstructured)
		if !ok {
			continue
		}

		resourceKey := formatPlanResourceKey(desiredUns)

		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(desiredUns.GroupVersionKind())
		err := t.k8sClient.Get(ctx, client.ObjectKey{
			Namespace: desiredUns.GetNamespace(),
			Name:      desiredUns.GetName(),
		}, live)

		if err != nil {
			if apierrors.IsNotFound(err) {
				changes = append(changes, planChange{
					kind:        planChangeCreate,
					resourceKey: resourceKey,
				})
				continue
			}
			return nil, errors.Wrapf(err, "failed to get live resource for %s", resourceKey)
		}

		// 比較に不要な server-managed フィールドを落として diff のノイズを減らす。
		// resourceVersion / uid などは apply に直接関係しないため。
		liveForDiff := sanitizeForDiff(live)
		desiredForDiff := sanitizeForDiff(desiredUns)

		diff := utildiff.Diff(liveForDiff.Object, desiredForDiff.Object)
		if strings.TrimSpace(diff) == "" {
			continue
		}

		changes = append(changes, planChange{
			kind:        planChangeUpdate,
			resourceKey: resourceKey,
			diff:        diff,
		})
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].kind != changes[j].kind {
			return planChangeOrder(changes[i].kind) < planChangeOrder(changes[j].kind)
		}
		return changes[i].resourceKey < changes[j].resourceKey
	})

	return changes, nil
}

// sanitizeForDiff は diff の前にライブオブジェクトから server-side で付与される
// 比較ノイズを取り除いた写しを返す。元のオブジェクトは変更しない。
func sanitizeForDiff(in *unstructured.Unstructured) *unstructured.Unstructured {
	out := in.DeepCopy()
	out.SetResourceVersion("")
	out.SetUID("")
	out.SetGeneration(0)
	// status は live にしか付かないことが多く desired との比較でノイズになるので落とす。
	unstructured.RemoveNestedField(out.Object, "status")
	unstructured.RemoveNestedField(out.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(out.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(out.Object, "metadata", "selfLink")
	return out
}

// formatPlanResourceKey はリソース表示用のキーを返す。
// namespaced: "Kind/namespace/name", cluster-scoped: "Kind/name"
func formatPlanResourceKey(u *unstructured.Unstructured) string {
	kind := u.GetKind()
	ns := u.GetNamespace()
	name := u.GetName()
	if ns == "" {
		return fmt.Sprintf("%s/%s", kind, name)
	}
	return fmt.Sprintf("%s/%s/%s", kind, ns, name)
}

// planChangeOrder は planChange を安定ソートするための優先度を返す。
// create -> update の順で並べる。
func planChangeOrder(k planChangeKind) int {
	switch k {
	case planChangeCreate:
		return 0
	case planChangeUpdate:
		return 1
	default:
		return 2
	}
}

// writePlanManifestHeader は manifest 単位のヘッダを書き出す。
func writePlanManifestHeader(w io.Writer, name string) error {
	if _, err := fmt.Fprintf(w, "Manifest: %s\n", name); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// writePlanChange は 1 件の planChange を出力する。
//   - create: "  + <Kind/ns/name> (to be created)"
//   - update: "  ~ <Kind/ns/name>" の直後に util/diff.Diff の unified diff をインデントして出力
func writePlanChange(w io.Writer, ch planChange) error {
	switch ch.kind {
	case planChangeCreate:
		if _, err := fmt.Fprintf(w, "  + %s (to be created)\n", ch.resourceKey); err != nil {
			return errors.WithStack(err)
		}
	case planChangeUpdate:
		if _, err := fmt.Fprintf(w, "  ~ %s\n", ch.resourceKey); err != nil {
			return errors.WithStack(err)
		}
		// util/diff.Diff の出力をそのまま流すと先頭行が "--- a..." 形式で長くなるので、
		// 行頭にインデントを付けて読みやすくする。空行は潰す。
		for _, line := range strings.Split(strings.TrimRight(ch.diff, "\n"), "\n") {
			if line == "" {
				continue
			}
			if _, err := fmt.Fprintf(w, "    %s\n", line); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}
