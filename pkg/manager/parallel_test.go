package manager

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

type fakeManager struct {
	applyErr   error
	destroyErr error
	buildOut   string
	buildErr   error
}

func (f *fakeManager) Apply(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return f.applyErr
}

func (f *fakeManager) Destroy(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return f.destroyErr
}

func (f *fakeManager) Build(_ context.Context, _ *slog.Logger, _ v1.Manifest) (string, error) {
	return f.buildOut, f.buildErr
}

func TestParallelApply(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	t.Run("nil parallel returns nil", func(t *testing.T) {
		p := NewParallel(nil)
		if err := p.Apply(ctx, logger, v1.Manifest{}); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("all children succeed", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeKustomize, Path: "b"},
				},
			},
		}
		if err := p.Apply(ctx, logger, m); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("some children fail", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{applyErr: errors.New("apply failed")},
			"helmfile":  &fakeManager{},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		err := p.Apply(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "apply failed") {
			t.Fatalf("expected error to contain 'apply failed', got %v", err)
		}
	})

	t.Run("all children fail", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{applyErr: errors.New("fail1")},
			"helmfile":  &fakeManager{applyErr: errors.New("fail2")},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		err := p.Apply(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "fail1") || !strings.Contains(err.Error(), "fail2") {
			t.Fatalf("expected both errors, got %v", err)
		}
	})

	t.Run("unknown manager type", func(t *testing.T) {
		p := NewParallel(map[string]Manager{})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: "nonexistent", Path: "a"},
				},
			},
		}
		err := p.Apply(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown manager type") {
			t.Fatalf("expected 'unknown manager type' error, got %v", err)
		}
	})
}

func TestParallelDestroy(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	t.Run("nil parallel returns nil", func(t *testing.T) {
		p := NewParallel(nil)
		if err := p.Destroy(ctx, logger, v1.Manifest{}); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("all children succeed", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
				},
			},
		}
		if err := p.Destroy(ctx, logger, m); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("some children fail", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{destroyErr: errors.New("destroy failed")},
			"helmfile":  &fakeManager{},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		err := p.Destroy(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "destroy failed") {
			t.Fatalf("expected error to contain 'destroy failed', got %v", err)
		}
	})

	t.Run("unknown manager type", func(t *testing.T) {
		p := NewParallel(map[string]Manager{})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: "nonexistent", Path: "a"},
				},
			},
		}
		err := p.Destroy(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown manager type") {
			t.Fatalf("expected 'unknown manager type' error, got %v", err)
		}
	})
}

func TestParallelBuild(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	t.Run("nil parallel returns empty", func(t *testing.T) {
		p := NewParallel(nil)
		out, err := p.Build(ctx, logger, v1.Manifest{})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if out != "" {
			t.Fatalf("expected empty output, got %q", out)
		}
	})

	t.Run("all children succeed and outputs are joined in order", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{buildOut: "A"},
			"helmfile":  &fakeManager{buildOut: "B"},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		out, err := p.Build(ctx, logger, m)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if out != "A\n---\nB" {
			t.Fatalf("expected 'A\\n---\\nB', got %q", out)
		}
	})

	t.Run("empty outputs are skipped", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{buildOut: "A"},
			"helmfile":  &fakeManager{buildOut: ""},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		out, err := p.Build(ctx, logger, m)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if out != "A" {
			t.Fatalf("expected 'A', got %q", out)
		}
	})

	t.Run("some children fail", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{buildErr: errors.New("build failed")},
			"helmfile":  &fakeManager{buildOut: "B"},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		_, err := p.Build(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "build failed") {
			t.Fatalf("expected error to contain 'build failed', got %v", err)
		}
	})

	t.Run("all children fail", func(t *testing.T) {
		p := NewParallel(map[string]Manager{
			"kustomize": &fakeManager{buildErr: errors.New("fail1")},
			"helmfile":  &fakeManager{buildErr: errors.New("fail2")},
		})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: v1.ManifestTypeKustomize, Path: "a"},
					{Type: v1.ManifestTypeHelmfile, Path: "b"},
				},
			},
		}
		_, err := p.Build(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "fail1") || !strings.Contains(err.Error(), "fail2") {
			t.Fatalf("expected both errors, got %v", err)
		}
	})

	t.Run("unknown manager type", func(t *testing.T) {
		p := NewParallel(map[string]Manager{})
		m := v1.Manifest{
			Parallel: &v1.ManifestParallel{
				Children: []v1.Manifest{
					{Type: "nonexistent", Path: "a"},
				},
			},
		}
		_, err := p.Build(ctx, logger, m)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown manager type") {
			t.Fatalf("expected 'unknown manager type' error, got %v", err)
		}
	})
}
