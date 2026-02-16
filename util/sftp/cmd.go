package sftp

import (
	"errors"
	"os"

	"github.com/pkg/sftp"
)

func (fs FishlerFS) Filecmd(request *sftp.Request) error {
	fs.logInfo(request, "sftp filecmd")

	p, err := fs.GetDockerVolumnPath(fs, request.Filepath)
	if err != nil {
		fs.logError(request, "sftp filecmd error", err)
		return sftp.ErrSSHFxNoSuchFile
	}

	var target string = ""

	if request.Target != "" {
		target, err = fs.GetDockerVolumnPath(fs, request.Target)
		if err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxOpUnsupported
		}
	}

	switch request.Method {
	case "Setstat":
		var mode os.FileMode = 0644

		if request.Attributes().FileMode().Perm() != 0000 {
			mode = request.Attributes().FileMode().Perm()
		}

		if request.Attributes().FileMode().IsDir() {
			mode = 0755
		}

		if err := os.Chmod(p, mode); err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}
		return nil
	case "Rename":
		if err := os.Rename(p, target); err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		return nil
	case "Remove":
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return sftp.ErrSSHFxNoSuchFile
			}
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		if info.IsDir() {
			return sftp.ErrSSHFxFailure
		}

		if err := os.Remove(p); err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		return sftp.ErrSSHFxOk

	case "Rmdir":

		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return sftp.ErrSSHFxNoSuchFile
			}
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		if !info.IsDir() {
			return sftp.ErrSSHFxFailure
		}

		if err := os.RemoveAll(p); err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		return sftp.ErrSSHFxOk

	case "Mkdir":
		if err := os.MkdirAll(p, 0750); err != nil {
			fs.logError(request, "sftp filecmd error", err)
			return sftp.ErrSSHFxFailure
		}

		return nil
	case "Symlink":
		if err := os.Symlink(p, target); err != nil {
			return sftp.ErrSSHFxFailure
		}

		return nil
	default:
		fs.logError(request, "sftp filecmd error", errors.New("unknown SFTP method requested"))
		return sftp.ErrSSHFxOpUnsupported
	}
}
