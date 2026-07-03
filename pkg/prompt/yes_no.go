package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
)

// YesORNo はyes/noの確認プロンプトを表示して回答を返す。
// 破壊的操作の確認に使われるため、空入力 (Enterのみ) はNo扱いにする。
func YesORNo(
	readFrom io.Reader,
	questionMessage string,
) (bool, error) {
	reader := bufio.NewReader(readFrom)

	fmt.Printf("%s: [y/N] ", questionMessage)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, errors.WithStack(err)
	}

	response = strings.TrimSpace(response)
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes"), nil
}
