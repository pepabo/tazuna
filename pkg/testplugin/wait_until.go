package testplugin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/google/cel-go/cel"
	v1 "github.com/pepabo/tazuna/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WaitUntil struct {
	client client.Client

	// prgMu / prgCache はコンパイル済みCELプログラムのキャッシュ。
	// Runはポーリングループから同じ式で繰り返し呼ばれるため、
	// 式ごとに一度だけコンパイルして再利用する。
	prgMu    sync.Mutex
	prgCache map[string]cel.Program
}

// Run implements Plugin.
func (w *WaitUntil) Run(
	ctx context.Context,
	logger *slog.Logger,
	spec *v1.TestPluginSpec,
) error {
	args := spec.WaitUntil
	if args == nil {
		return fmt.Errorf(".spec.waitUntil is not defined")
	}

	gv, err := schema.ParseGroupVersion(args.Resource.APIVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	gvk := gv.WithKind(args.Resource.Kind)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := client.ObjectKey{
		Namespace: args.Namespace,
		Name:      args.Name,
	}
	if err := w.client.Get(ctx, key, obj); err != nil {
		return errors.WithStack(err)
	}

	prg, err := w.program(args.Condition)
	if err != nil {
		return err
	}

	result, err := evaluateCELProgram(prg, obj.Object)
	if err != nil {
		return err
	}

	if !result {
		return fmt.Errorf("condition %q is not satisfied (resource: %s/%s)", args.Condition, args.Namespace, args.Name)
	}

	return nil
}

// Type implements Plugin.
func (w *WaitUntil) Type() v1.TestPluginType {
	return v1.TestPluginTypeWaitUntil
}

func NewWaitUntil(c client.Client) *WaitUntil {
	return &WaitUntil{client: c}
}

var _ Plugin = &WaitUntil{}

// program は式に対応するコンパイル済みCELプログラムを返す。
// 初回のみコンパイルし、以降はキャッシュを返す。
func (w *WaitUntil) program(expression string) (cel.Program, error) {
	w.prgMu.Lock()
	defer w.prgMu.Unlock()

	if prg, ok := w.prgCache[expression]; ok {
		return prg, nil
	}

	prg, err := CompileCEL(expression)
	if err != nil {
		return nil, err
	}

	if w.prgCache == nil {
		w.prgCache = map[string]cel.Program{}
	}
	w.prgCache[expression] = prg
	return prg, nil
}

// CompileCEL はCEL式をコンパイルしてプログラムを返す。
// 出力型はboolに制限される。
func CompileCEL(expression string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable("object", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("CEL expression result is not a bool type: %s", ast.OutputType())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	return prg, nil
}

// evaluateCELProgram はコンパイル済みCELプログラムをobjectに対して評価する。
func evaluateCELProgram(prg cel.Program, object map[string]any) (bool, error) {
	out, _, err := prg.Eval(map[string]any{
		"object": object,
	})
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("could not convert CEL expression result to bool")
	}

	return result, nil
}

// EvaluateCEL はCEL式をコンパイルして評価し、結果をboolで返す。
// ポーリングループから呼ぶ場合はコンパイル結果を再利用できる
// CompileCEL + evaluateCELProgram の経路を使うこと。
func EvaluateCEL(expression string, object map[string]any) (bool, error) {
	prg, err := CompileCEL(expression)
	if err != nil {
		return false, err
	}
	return evaluateCELProgram(prg, object)
}
