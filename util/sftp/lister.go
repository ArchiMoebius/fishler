package sftp

import (
	"io"
	"os"
)

type ListerAt []os.FileInfo

func (l ListerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}

	if n := copy(f, l[offset:]); n < len(f) {
		return n, io.EOF
	} else {
		return n, nil
	}
}
