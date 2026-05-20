package dispatch

import (
	"bytes"
	"fmt"
	"io"
)

// linePrefixWriter is an io.Writer that emits each completed `\n`-terminated
// line of input to out prefixed with prefix, and tees the unprefixed line to
// log when log is non-nil. Partial lines are buffered until the next newline
// arrives. The buffer grows unbounded so a single large stream-json event
// embedding a big tool result is never split across two prefixed output lines.
//
// Intended use: set as cmd.Stdout, then call Flush after cmd.Wait so any
// trailing partial line (output without a final \n before exit) is not lost.
// os/exec serializes calls to a single Stdout writer, so the buf field needs
// no synchronization.
type linePrefixWriter struct {
	prefix string
	out    io.Writer
	log    io.Writer
	buf    []byte
}

func (w *linePrefixWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		nl := bytes.IndexByte(w.buf, '\n')
		if nl < 0 {
			break
		}
		line := w.buf[:nl]
		fmt.Fprintln(w.out, w.prefix+string(line))
		if w.log != nil {
			fmt.Fprintln(w.log, string(line))
		}
		w.buf = w.buf[nl+1:]
	}
	return len(p), nil
}

func (w *linePrefixWriter) Flush() {
	if len(w.buf) == 0 {
		return
	}
	fmt.Fprintln(w.out, w.prefix+string(w.buf))
	if w.log != nil {
		fmt.Fprintln(w.log, string(w.buf))
	}
	w.buf = nil
}
