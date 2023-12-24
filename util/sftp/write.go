package sftp

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

func (fs FishlerFS) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	p, err := fs.GetDockerVolumnPath(fs, request.Filepath, false)
	if err != nil {
		fs.logError(request, "sftp write error", err)
		return nil, sftp.ErrSSHFxNoSuchFile
	}

	if !fs.HasDiskSpace(fs) {
		fs.logError(request, "sftp write error", errors.New("out of disk space"))
		// https://tools.ietf.org/html/draft-ietf-secsh-filexfer-13#section-9.1 > sshFxQuotaExceeded...
		return nil, sftp.ErrSSHFxFailure
	}

	fs.Lock.Lock()
	defer fs.Lock.Unlock()

	stat, statErr := os.Stat(p)

	if os.IsNotExist(statErr) {
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			fs.logError(request, "sftp write error", err)
			return nil, sftp.ErrSSHFxFailure
		}

		file, err := os.Create(p) // #nosec
		if err != nil {
			fs.logError(request, "sftp write error", err)
			return nil, sftp.ErrSSHFxFailure
		}

		return file, nil
	}

	if statErr != nil {
		return nil, sftp.ErrSSHFxFailure
	}

	if stat.IsDir() {
		return nil, sftp.ErrSSHFxOpUnsupported
	}

	file, err := os.Create(p) // #nosec
	if err != nil {
		fs.logError(request, "sftp write error", err)
		return nil, sftp.ErrSSHFxFailure
	}

	fs.logInfo(request, "sftp write")

	return file, nil
}
