package testplugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cockroachdb/errors"
	"github.com/google/cel-go/cel"
	v1 "github.com/pepabo/tazuna/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WaitUntil struct {
	client client.Client
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

	result, err := EvaluateCEL(args.Condition, obj.Object)
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
	return &WaitUntil{c}
}

var _ Plugin = &WaitUntil{}

// evaluateCEL はCEL式を評価し、結果をboolで返す
func EvaluateCEL(expression string, object map[string]interface{}) (bool, error) {
	env, err := cel.NewEnv(
		cel.Variable("object", cel.DynType),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	if ast.OutputType() != cel.BoolType {
		return false, fmt.Errorf("CEL expression result is not a bool type: %s", ast.OutputType())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL program: %w", err)
	}

	out, _, err := prg.Eval(map[string]interface{}{
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
