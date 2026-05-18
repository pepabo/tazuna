package genesissecret

import (
	"context"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// SecretProvider はGenesisSecretが秘匿情報を取得する先を抽象化して提供します
// NOTE: SecretProvider[Arg comparable] というジェネリクスを利用して、
//
//	各Providerが受け取る型を制約付ける実装を検討しましたが、
//	Fetch時に渡すargがManagerからみて決定的でないため、採用しないことにしました
type SecretProvider interface {
	// Fetch はProviderを経由して秘匿情報のアイテムを取得します
	Fetch(ctx context.Context, s v1.GenesisSecretGenerate) (map[string]string, error)
}
