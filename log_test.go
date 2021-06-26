package echo

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

type bufferLogger struct {
	buffer *bytes.Buffer
}

func newBufferLogger() *bufferLogger {
	return &bufferLogger{
		buffer: new(bytes.Buffer),
	}
}

func (l *bufferLogger) Writer() io.Writer {
	return bufio.NewWriter(l.buffer)
}

func (l *bufferLogger) Printf(format string, args ...interface{}) {
	l.buffer.WriteString(fmt.Sprintf(format, args...))
}

func (l *bufferLogger) Error(err error) {
	l.buffer.WriteString(err.Error())
}
