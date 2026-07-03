package genesissecret

import (
	"net/url"
	"strings"

	"github.com/cockroachdb/errors"
)

// ParseOnePasswordURI は `op://<host>/<vault>/<item>` 形式の URI をパースして
// vault と item を返します。op CLI で一般的な `op://<vault>/<item>` 形式は
// host 部が欠けているため明示的にエラーとします。
func ParseOnePasswordURI(uri string) (vault, item string, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", errors.Wrapf(err, "invalid 1Password URI %q", uri)
	}
	if u.Scheme != "op" {
		return "", "", errors.Errorf("1Password URI %q must use op:// scheme, expected format is op://<host>/<vault>/<item>", uri)
	}

	v := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if u.Host == "" || len(v) != 2 || v[0] == "" || v[1] == "" {
		return "", "", errors.Errorf("1Password URI %q is malformed, expected format is op://<host>/<vault>/<item>", uri)
	}

	return v[0], v[1], nil
}
