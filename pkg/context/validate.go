package context

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// ValidateCurrentContext は現在のkubeconfigコンテキスト名がcontextMatchesパターンにマッチするか検証します
func ValidateCurrentContext(patterns []string, mode v1.ContextMatchMode) error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		return errors.Wrap(err, "failed to load kubeconfig")
	}

	return matchContext(config.CurrentContext, patterns, mode)
}

func matchContext(currentContext string, patterns []string, mode v1.ContextMatchMode) error {
	if mode == "" {
		mode = v1.ContextMatchModeOR
	}

	switch mode {
	case v1.ContextMatchModeOR:
		for _, pattern := range patterns {
			matched, err := regexp.MatchString(pattern, currentContext)
			if err != nil {
				return errors.Errorf("failed to match context_matches pattern %q: %s", pattern, err)
			}
			if matched {
				return nil
			}
		}
		return errors.Errorf("current context %q does not match any of context_matches patterns [%s]",
			currentContext, joinPatterns(patterns))
	case v1.ContextMatchModeAND:
		var unmatched []string
		for _, pattern := range patterns {
			matched, err := regexp.MatchString(pattern, currentContext)
			if err != nil {
				return errors.Errorf("failed to match context_matches pattern %q: %s", pattern, err)
			}
			if !matched {
				unmatched = append(unmatched, pattern)
			}
		}
		if len(unmatched) > 0 {
			return errors.Errorf("current context %q does not match context_matches patterns [%s]",
				currentContext, joinPatterns(unmatched))
		}
		return nil
	default:
		return errors.Errorf("unsupported context_match_mode: %s", mode)
	}
}

func joinPatterns(patterns []string) string {
	quoted := make([]string, len(patterns))
	for i, p := range patterns {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(quoted, ", ")
}
