package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/cockroachdb/errors"
)

func YesORNo(
	readFrom io.Reader,
	questionMessage string,
) (bool, error) {
	reader := bufio.NewReader(readFrom)

	fmt.Printf("%s: [Y/n] ", questionMessage)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, errors.WithStack(err)
	}

	response = strings.TrimSpace(response)
	return strings.EqualFold(response, "y") || response == "", nil
}
