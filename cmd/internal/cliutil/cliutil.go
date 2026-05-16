// Package cliutil collects boilerplate shared across cobra commands in cmd/.
// Each helper here is deliberately small and orthogonal so that RunE bodies can
// focus on command-specific orchestration rather than plumbing.
package cliutil

import (
	"log/slog"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ParseLogLevel maps the textual log level used by Tazuna's --log-level flag to
// a slog.Level. Unknown values fall back to slog.LevelInfo.
func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger reads the persistent --log-level flag and returns a slog.Logger
// that writes to stderr.
func NewLogger(cmd *cobra.Command) (*slog.Logger, error) {
	logLevelS, err := cmd.Flags().GetString("log-level")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: ParseLogLevel(logLevelS)})), nil
}

// LoadTazunaYAML opens path, decodes it as a v1.Tazuna document, and closes
// the file. Decoding and close errors are joined so neither is dropped.
func LoadTazunaYAML(path string) (_ *v1.Tazuna, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, errors.WithStack(cerr))
		}
	}()

	tazuna := v1.Tazuna{}
	if err := yaml.NewDecoder(f).Decode(&tazuna); err != nil {
		return nil, errors.WithStack(err)
	}
	return &tazuna, nil
}

// NewK8sClient builds a controller-runtime client from the ambient kubeconfig.
func NewK8sClient() (client.Client, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}
