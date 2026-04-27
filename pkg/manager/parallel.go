package manager

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

type Parallel struct {
	Managers map[string]Manager
}

func NewParallel(managers map[string]Manager) *Parallel {
	return &Parallel{
		Managers: managers,
	}
}

// Apply implements Manager.
func (p *Parallel) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	if m.Parallel == nil {
		return nil
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(m.Parallel.Children))
	errCh := make(chan error, len(m.Parallel.Children))
	logger.InfoContext(ctx, "applying parallel manifests", slog.Int("count", len(m.Parallel.Children)))

	for _, child := range m.Parallel.Children {
		go func(child v1.Manifest) {
			defer wg.Done()
			childManager, ok := p.Managers[string(child.Type)]
			if !ok {
				logger.ErrorContext(ctx, "unknown manager type", slog.String("type", string(child.Type)))
				errCh <- errors.Newf("unknown manager type: %s", child.Type)
				return
			}

			if err := childManager.Apply(ctx, logger, child); err != nil {
				logger.ErrorContext(ctx, "failed to apply child manifest", slog.String("path", child.Path), slog.Any("error", err))
				errCh <- errors.Wrapf(err, "failed to apply child manifest: %s", child.Path)
				return
			}
		}(child)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Build implements Manager.
func (p *Parallel) Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error) {
	if m.Parallel == nil {
		return "", nil
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(m.Parallel.Children))
	errCh := make(chan error, len(m.Parallel.Children))
	outputs := make([]string, len(m.Parallel.Children))
	logger.InfoContext(ctx, "building parallel manifests", slog.Int("count", len(m.Parallel.Children)))

	for i, child := range m.Parallel.Children {
		go func(i int, child v1.Manifest) {
			defer wg.Done()
			childManager, ok := p.Managers[string(child.Type)]
			if !ok {
				logger.ErrorContext(ctx, "unknown manager type", slog.String("type", string(child.Type)))
				errCh <- errors.Newf("unknown manager type: %s", child.Type)
				return
			}

			out, err := childManager.Build(ctx, logger, child)
			if err != nil {
				logger.ErrorContext(ctx, "failed to build child manifest", slog.String("path", child.Path), slog.Any("error", err))
				errCh <- errors.Wrapf(err, "failed to build child manifest: %s", child.Path)
				return
			}
			outputs[i] = out
		}(i, child)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	var nonEmpty []string
	for _, out := range outputs {
		if out != "" {
			nonEmpty = append(nonEmpty, out)
		}
	}
	return strings.Join(nonEmpty, "\n---\n"), nil
}

// Destroy implements Manager.
func (p *Parallel) Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error {
	if m.Parallel == nil {
		return nil
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(m.Parallel.Children))
	errCh := make(chan error, len(m.Parallel.Children))
	logger.InfoContext(ctx, "destroying parallel manifests", slog.Int("count", len(m.Parallel.Children)))

	for _, child := range m.Parallel.Children {
		go func(child v1.Manifest) {
			defer wg.Done()
			childManager, ok := p.Managers[string(child.Type)]
			if !ok {
				logger.ErrorContext(ctx, "unknown manager type", slog.String("type", string(child.Type)))
				errCh <- errors.Newf("unknown manager type: %s", child.Type)
				return
			}

			if err := childManager.Destroy(ctx, logger, child); err != nil {
				logger.ErrorContext(ctx, "failed to destroy child manifest", slog.String("path", child.Path), slog.Any("error", err))
				errCh <- errors.Wrapf(err, "failed to destroy child manifest: %s", child.Path)
				return
			}
		}(child)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

var _ Manager = &Parallel{}
