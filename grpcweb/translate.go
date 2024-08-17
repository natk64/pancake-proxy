package grpcweb

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"strings"

	"github.com/natk64/pancake-proxy/utils"
)

const (
	ContentTypeGrpc        = "application/grpc"
	ContentTypeGrpcWeb     = "application/grpc-web"
	ContentTypeGrpcWebText = "application/grpc-web-text"
)

type Finisher interface {
	Finish()
}

func WrapRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *http.Request) {
	contentType := r.Header.Get("Content-Type")
	baseContentType := ContentTypeGrpcWeb

	if strings.HasPrefix(contentType, ContentTypeGrpcWebText) {
		r.Body = utils.CombineReaderCloser(base64.NewDecoder(base64.StdEncoding, r.Body), r.Body)
		w = utils.Base64ResponseWriter(w)
		baseContentType = ContentTypeGrpcWebText
	}

	header := make(http.Header)
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		header.Set("Content-Encoding", "gzip")
		w = utils.GzipResponseWriter(w)
	}

	// Replace just part of the content type, to make sure the message format is retained.
	r.Header.Set("Content-Type", strings.Replace(contentType, baseContentType, ContentTypeGrpc, 1))
	r.Header.Del("Content-Length")
	r.ContentLength = -1
	r.ProtoMajor = 2
	r.ProtoMinor = 0

	w = &grpcWebResponseWriter{
		inner:           w,
		headers:         header,
		baseContentType: baseContentType,
	}

	return w, r
}

var _ http.ResponseWriter = (*grpcWebResponseWriter)(nil)

type grpcWebResponseWriter struct {
	headers         http.Header
	headersCopied   bool
	baseContentType string

	inner http.ResponseWriter
}

// Header implements http.ResponseWriter.
func (g *grpcWebResponseWriter) Header() http.Header {
	return g.headers
}

// Write implements http.ResponseWriter.
func (g *grpcWebResponseWriter) Write(data []byte) (int, error) {
	if !g.headersCopied {
		g.CopyHeaders()
	}
	return g.inner.Write(data)
}

// WriteHeader implements http.ResponseWriter.
func (g *grpcWebResponseWriter) WriteHeader(statusCode int) {
	g.CopyHeaders()
	g.inner.WriteHeader(statusCode)
}

func (g *grpcWebResponseWriter) Flush() {
	if f, ok := g.inner.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *grpcWebResponseWriter) CopyHeaders() {
	target := g.inner.Header()
	for key, value := range g.headers {
		lower := strings.ToLower(key)
		if lower == "trailer" {
			continue
		}

		if lower == "content-type" {
			for i, old := range value {
				if old == ContentTypeGrpc || strings.HasPrefix(old, ContentTypeGrpc+"+") {
					value[i] = g.baseContentType + old[len(ContentTypeGrpc):]
				}
			}
		}

		target[key] = value
	}

	g.headersCopied = true

}

// Finish finishes the gRPC request by writing the trailers to the response body.
func (g *grpcWebResponseWriter) Finish() {
	trailers := make(http.Header)
	sentHeaders := make(map[string]bool)
	for key := range g.inner.Header() {
		sentHeaders[strings.ToLower(key)] = true
	}

	// Find all headers that haven't been sent yet.
	for key, value := range g.headers {
		key = strings.ToLower(key)
		if sentHeaders[key] || key == "trailer" {
			continue
		}

		key = strings.TrimPrefix(key, http.TrailerPrefix)
		trailers[key] = value
	}

	buf := &bytes.Buffer{}
	trailers.Write(buf)
	frameHeader := make([]byte, 5)
	frameHeader[0] = 0b10000000
	binary.BigEndian.PutUint32(frameHeader[1:], uint32(buf.Len()))
	g.inner.Write(frameHeader)
	g.inner.Write(buf.Bytes())
	g.Flush()
}
