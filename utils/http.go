package utils

import (
	"encoding/base64"
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

type ResponseWriterFlusher interface {
	http.ResponseWriter
	http.Flusher
}

type base64ResponseWriter struct {
	inner          http.ResponseWriter
	currentEncoder io.WriteCloser
}

func (b *base64ResponseWriter) Header() http.Header {
	return b.inner.Header()
}

func (b *base64ResponseWriter) Write(data []byte) (int, error) {
	return b.currentEncoder.Write(data)
}

func (b *base64ResponseWriter) WriteHeader(statusCode int) {
	b.inner.WriteHeader(statusCode)
}

func (b *base64ResponseWriter) Flush() {
	// There's no other way to flush a base64.Encoder.
	b.currentEncoder.Close()
	b.currentEncoder = base64.NewEncoder(base64.RawStdEncoding, b.inner)
	if f, ok := b.inner.(http.Flusher); ok {
		f.Flush()
	}
}

// Base64ResponseWriter wraps an http.ResponseWriter that encodes data to base64.StdEncoding.
// The returned flusher implements http.Flusher.
func Base64ResponseWriter(w http.ResponseWriter) ResponseWriterFlusher {
	return &base64ResponseWriter{
		inner:          w,
		currentEncoder: base64.NewEncoder(base64.RawStdEncoding, w),
	}
}
