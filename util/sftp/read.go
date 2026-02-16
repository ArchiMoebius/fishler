package sftp

import (
	"io"
	"os"

	"github.com/pkg/sftp"
)

func (fs FishlerFS) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	p, err := fs.GetDockerVolumnPath(fs, request.Filepath)
	if err != nil {
		return nil, sftp.ErrSSHFxNoSuchFile
	}

	fs.Lock.Lock()
	defer fs.Lock.Unlock()

	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, sftp.ErrSSHFxNoSuchFile
	}

	file, err := os.Open(p) // #nosec
	if err != nil {
		return nil, sftp.ErrSSHFxFailure
	}

	return file, nil
}
