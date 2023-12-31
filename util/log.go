package util

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/snowzach/rotatefilehook"
)

var Logger *logrus.Logger

// FormatterHook is a hook that writes logs of specified LogLevels with a formatter to specified Writer
type FormatterHook struct {
	Writer    io.Writer
	LogLevels []logrus.Level
	Formatter logrus.Formatter
}

// Fire will be called when some logging function is called with current hook
// It will format log entry and write it to appropriate writer
func (hook *FormatterHook) Fire(entry *logrus.Entry) error {
	line, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.Writer.Write(line)
	return err
}

// Levels define on which log levels this hook would trigger
func (hook *FormatterHook) Levels() []logrus.Level {
	return hook.LogLevels
}

func SetLogger(filepath string) *logrus.Logger {
	systemlog, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640) // #nosec

	if err != nil {
		log.Printf("Failed to create logfile %s\n", filepath)
		return nil
	}

	logger := logrus.New()

	rotateFileHook, err := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
		Filename:   filepath,
		MaxSize:    50, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Level:      logrus.DebugLevel,
		Formatter: &logrus.JSONFormatter{
			TimestampFormat: time.DateTime,
		},
	})

	if err != nil {
		log.Printf("Failed to create logfile %s\n", filepath)
		return nil
	}

	logger.SetOutput(io.Discard) // Send all logs to nowhere by default
	logger.SetLevel(logrus.DebugLevel)

	logger.ReportCaller = false

	logger.AddHook(&FormatterHook{ // Send logs with level higher than info to systemlog
		Writer: systemlog,
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
		},
		Formatter: &logrus.JSONFormatter{},
	})
	logger.AddHook(&FormatterHook{
		Writer: os.Stderr,
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
			logrus.DebugLevel,
			logrus.TraceLevel,
		},
		Formatter: &logrus.TextFormatter{
			TimestampFormat: time.DateTime,
			FullTimestamp:   true,
			ForceColors:     true,
		},
	})

	logger.AddHook(rotateFileHook)

	Logger = logger

	return logger
}

func GetSessionVolumnDirectory(basepath string, directory string, sessionRemoteAddress net.Addr) string {
	ipaddr := strings.ReplaceAll(strings.ReplaceAll(strings.Split(sessionRemoteAddress.String(), ":")[0], ".", "-"), ":", "_")
	dirpath := fmt.Sprintf("%s/%s/%s", basepath, directory, ipaddr)

	err := os.MkdirAll(dirpath, 0750)
	if err != nil {
		log.Fatal(err)
	}

	return dirpath
}

func GetSessionFileName(basepath string, containerID string, sessionRemoteAddress net.Addr) string {
	datetime := strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", time.Now().Format(time.RFC3339)), ":", ""), "-", "")
	ipaddr := strings.ReplaceAll(strings.ReplaceAll(sessionRemoteAddress.String(), ".", "-"), ":", "_")

	err := os.MkdirAll(basepath, 0750)
	if err != nil {
		log.Fatal(err)
	}

	return fmt.Sprintf("%s/%s_%s_%s.log", basepath, containerID, datetime, ipaddr)
}

func ByteCountDecimal(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}
