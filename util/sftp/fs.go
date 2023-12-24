package sftp

import (
	"sync"

	"github.com/pkg/sftp"
	"github.com/sirupsen/logrus"

	"github.com/archimoebius/fishler/util"
)

type FishlerFS struct {
	GetDockerVolumnPath func(fs FishlerFS, p string, trash bool) (string, error)
	HasDiskSpace        func(fs FishlerFS) bool
	Lock                *sync.Mutex
	User                string
	RemoteIP            string
}

func (fs FishlerFS) logError(request *sftp.Request, msg string, err error) {
	util.Logger.WithFields(logrus.Fields{
		"address": fs.RemoteIP,
		"user":    fs.User,
		"tpath":   request.Target,
		"rpath":   request.Filepath,
		"method":  request.Method,
		"error":   err,
	}).Error(msg)
}

func (fs FishlerFS) logInfo(request *sftp.Request, msg string) {
	util.Logger.WithFields(logrus.Fields{
		"address": fs.RemoteIP,
		"user":    fs.User,
		"tpath":   request.Target,
		"rpath":   request.Filepath,
		"method":  request.Method,
	}).Info(msg)
}
