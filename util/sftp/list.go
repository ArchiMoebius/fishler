package sftp

import (
	"os"

	"github.com/pkg/sftp"
)

func (fs FishlerFS) Filelist(request *sftp.Request) (sftp.ListerAt, error) {
	p, err := fs.GetDockerVolumnPath(fs, request.Filepath, false)
	if err != nil {
		fs.logError(request, "sftp filelist error", err)
		return nil, sftp.ErrSSHFxNoSuchFile
	}

	fs.logInfo(request, "sftp filelist")

	switch request.Method {
	case "List":
		var filesinfo []os.FileInfo

		files, err := os.ReadDir(p)

		if err != nil {
			fs.logError(request, "sftp filelist error", err)
			return nil, sftp.ErrSSHFxFailure
		}

		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				fs.logError(request, "sftp filelist error", err)
				continue
			}
			filesinfo = append(filesinfo, info)
		}

		return ListerAt(filesinfo), nil
	case "Stat":
		s, err := os.Stat(p)

		if os.IsNotExist(err) {
			fs.logError(request, "sftp filelist error", err)
			return nil, sftp.ErrSSHFxNoSuchFile
		} else if err != nil {
			fs.logError(request, "sftp filelist error", err)
			return nil, sftp.ErrSSHFxFailure
		}

		return ListerAt([]os.FileInfo{s}), nil
	default:
	}

	return nil, sftp.ErrSSHFxOpUnsupported
}
