package main

import (
	"crypto/tls"
	"io"
	"log"
	"net/http"

	"golang.org/x/net/http2"
)

func main() {
	err := http.ListenAndServeTLS(":8081", "server.crt", "server.key", http.HandlerFunc(Handler))
	log.Fatalln(err)
}

const upstream = "localhost:5000"

func Handler(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	r.URL.Host = upstream
	r.URL.Scheme = "https"
	r.Host = upstream
	r.RequestURI = ""

	response, err := client.Do(r)
	if err != nil {
		log.Println("client.Do()", err)
		return
	}

	for key, values := range response.Header {
		for _, value := range values {
			log.Printf("%s: %s\n", key, value)
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
	w.WriteHeader(response.StatusCode)

	if _, err := io.Copy(autoFlusher{writer: w}, io.TeeReader(response.Body, log.Writer())); err != nil {
		log.Println("io.Copy()", err)
	}

	for key, values := range response.Trailer {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

type autoFlusher struct {
	writer io.Writer
}

func (a autoFlusher) Write(data []byte) (int, error) {
	n, err := a.writer.Write(data)
	if err != nil {
		return n, err
	}

	if n == 0 {
		return 0, nil
	}

	if flusher, ok := a.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, nil
}
