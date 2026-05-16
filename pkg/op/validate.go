package op

import (
	"regexp"

	"github.com/cockroachdb/errors"
)

// identifierPattern restricts vault / item identifiers to characters that the
// 1Password CLI handles unambiguously. Values outside this set tend to produce
// opaque `op` errors, so we reject them at the call site instead.
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.\- ]+$`)

// ValidateIdentifier checks that name is a non-empty identifier that matches
// the allowlist. kind is included in the error so callers can tell which
// argument was rejected (e.g. "vault", "item").
func ValidateIdentifier(kind, name string) error {
	if name == "" {
		return errors.Newf("%s identifier must not be empty", kind)
	}
	if !identifierPattern.MatchString(name) {
		return errors.Newf("%s identifier %q contains characters outside the allowed set %s", kind, name, identifierPattern.String())
	}
	return nil
}
