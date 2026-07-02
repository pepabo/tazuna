package cliutil

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// AtomicWriteFile はtemp file + renameでファイルをアトミックに書き込みます。
// 書き込み途中でプロセスが落ちても元ファイルが破損しないことを保証します。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.WithStack(err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if retErr != nil {
			if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
				retErr = errors.CombineErrors(retErr, err)
			}
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return errors.CombineErrors(errors.WithStack(err), tmp.Close())
	}
	if err := tmp.Chmod(perm); err != nil {
		return errors.CombineErrors(errors.WithStack(err), tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(os.Rename(tmpPath, path))
}
