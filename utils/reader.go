package utils

import "io"

type combinedCloser struct {
	r io.Reader
	c io.Closer
}

func (combined *combinedCloser) Close() error {
	return combined.c.Close()
}

func (combined *combinedCloser) Read(p []byte) (n int, err error) {
	return combined.r.Read(p)
}

// CombineReaderCloser combines an io.Reader and io.Closer into an io.ReadCloser
func CombineReaderCloser(reader io.Reader, closer io.Closer) io.ReadCloser {
	return &combinedCloser{
		r: reader,
		c: closer,
	}
}
