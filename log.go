package echo

import (
	"io"
	"log"
	"os"
)

// Logger defines the logging interface that Echo uses internally.
// For logging in handlers use your own logger instance (dependency injected or package/public variable) from logging framework of your choice.
type Logger interface {
	// Writer provides writer for http.Server `ErrorLog`. http.Server.ErrorLog logs errors from accepting connections, unexpected behavior
	// from handlers, and underlying FileSystem errors.
	Writer() io.Writer
	// Printf logs all non-error level events. In your own implementation choose level by your own liking (info/debug etc)
	Printf(format string, args ...interface{})
	// Error logs the error
	Error(err error)
}

type stdLogger struct {
	logger *log.Logger
}

func newStdLogger() *stdLogger {
	return &stdLogger{
		logger: log.New(os.Stdout, "echo: ", 0),
	}
}

func (l *stdLogger) Writer() io.Writer {
	return l.logger.Writer()
}

func (l *stdLogger) Printf(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

func (l *stdLogger) Error(err error) {
	l.logger.Print(err)
}
