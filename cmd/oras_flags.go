package cmd

import (
	"github.com/cockroachdb/errors"
	orasmanager "github.com/pepabo/tazuna/pkg/manager/oras"
	"github.com/spf13/cobra"
)

// addORASPullFlags は ORAS pull の挙動を制御する共通フラグをコマンドに追加します。
// apply / build / destroy で同一フラグを共有するため 1 か所に集約しています。
func addORASPullFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("no-cache", false, "Bypass the cache and always re-fetch from the registry on ORAS pull")
	cmd.Flags().Bool("offline", false, "Disallow registry access on ORAS pull (cache miss results in an error)")
}

// buildORASPullOptions は cobra コマンドから --no-cache / --offline を読み取り、
// 両立不可ならエラーを返します。
func buildORASPullOptions(cmd *cobra.Command) (orasmanager.PullOptions, error) {
	noCache, err := cmd.Flags().GetBool("no-cache")
	if err != nil {
		return orasmanager.PullOptions{}, errors.WithStack(err)
	}
	offline, err := cmd.Flags().GetBool("offline")
	if err != nil {
		return orasmanager.PullOptions{}, errors.WithStack(err)
	}
	if noCache && offline {
		return orasmanager.PullOptions{}, errors.New("--no-cache and --offline cannot be specified together")
	}
	return orasmanager.PullOptions{NoCache: noCache, Offline: offline}, nil
}
