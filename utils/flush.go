package utils

import (
	"io"
	"net/http"
)

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type autoFlusher struct {
	inner writeFlusher
}

// HttpAutoFlusher returns a writer that automatically calls [http.Flusher.Flush] after every successful call to [Write] (unless n is 0).
//
// If the writer doesn't implement http.Flusher, the same writer is returned as is.
func HttpAutoFlusher(rw io.Writer) io.Writer {
	flusher, ok := rw.(writeFlusher)
	if !ok {
		return rw
	}
	return autoFlusher{inner: flusher}
}

func (a autoFlusher) Write(data []byte) (n int, err error) {
	n, err = a.inner.Write(data)
	if err != nil {
		return n, err
	}

	if n != 0 {
		a.inner.Flush()
	}

	return n, nil
}
