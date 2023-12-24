package sftp

import (
	"io"

	"github.com/pkg/sftp"
)

func (fs FishlerFS) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	fs.logInfo(request, "sftp fileread")
	return nil, nil
	// p, err := fs.GetDockerVolumnPath(fs, request.Filepath, false)
	// if err != nil {
	// 	return nil, sftp.ErrSSHFxNoSuchFile
	// }

	// fs.Lock.Lock()
	// defer fs.Lock.Unlock()

	// if _, err := os.Stat(p); os.IsNotExist(err) {
	// 	return nil, sftp.ErrSSHFxNoSuchFile
	// }

	// file, err := os.Open(p)
	// if err != nil {
	// 	return nil, sftp.ErrSSHFxFailure
	// }

	// return file, nil
}
