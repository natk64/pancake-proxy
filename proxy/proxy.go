package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/natk64/pancake-proxy/grpcweb"
	"github.com/natk64/pancake-proxy/reflection"
	"github.com/natk64/pancake-proxy/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type upstreamServer struct {
	host               string
	plaintext          bool
	insecureSkipVerify bool

	reflectionClient *grpcreflect.Client
	httpClient       *http.Client
}

type upstreamService struct {
	servers []*upstreamServer
	next    atomic.Uint32
}

type proxy struct {
	upstreams []*upstreamServer

	reflectionResolver *reflection.SimpleResolver

	services      map[string]*upstreamService
	servicesMutex *sync.RWMutex

	internalServer *grpc.Server
	logger         *zap.Logger

	serviceUpdateInterval    time.Duration
	disableReflectionService bool
}

// RunBackgroundLoop MUST be called before the first request can be served.
// The function will block until the context is cancelled.
// This function will always return a non nil error.
func (p *proxy) RunProxy(ctx context.Context) error {
	ticker := time.NewTicker(p.serviceUpdateInterval)
	defer ticker.Stop()

	p.updateServices()

	for {
		select {
		case <-ticker.C:
			p.updateServices()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ServeHTTP implements the http.Handler interface.
// This method is the entrypoint for all requests into the proxy.
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	serviceName, ok := p.getTargetService(r)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "malformed request url")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, grpcweb.ContentTypeGrpcWeb) {
		w, r = grpcweb.WrapRequest(w, r)
	}

	if f, ok := w.(grpcweb.Finisher); ok {
		defer f.Finish()
	}

	w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")

	if p.handleReflection(w, r, serviceName) {
		return
	}

	server, ok := p.findServer(serviceName)
	if !ok {
		writeGrpcStatus(w, codes.Unimplemented, "no server provides the service")
		return
	}

	p.forwardRequest(r, w, server)
}

// findServer finds a server implementing the specified service using round robin load balancing.
func (p *proxy) findServer(serviceName string) (*upstreamServer, bool) {
	p.servicesMutex.RLock()
	defer p.servicesMutex.RUnlock()

	service, ok := p.services[serviceName]
	if !ok || len(service.servers) == 0 {
		return nil, false
	}

	next := int(service.next.Add(1))
	server := service.servers[next%len(service.servers)]
	return server, true
}

// getTargetService returns the name of the service this request is targeting.
func (p *proxy) getTargetService(r *http.Request) (name string, ok bool) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	split := strings.SplitN(path, "/", 2)
	if len(split) != 2 {
		return "", false
	}

	return split[0], true
}

// forwardRequest forwards an incoming gRPC request to the specified server.
func (p *proxy) forwardRequest(req *http.Request, w http.ResponseWriter, server *upstreamServer) {
	req.URL.Host = server.host
	req.Host = server.host
	req.RequestURI = ""

	if server.plaintext {
		req.URL.Scheme = "http"
	} else {
		req.URL.Scheme = "https"
	}

	response, err := server.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("Failed to start request", zap.Error(err))
		return
	}

	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(response.StatusCode)
	if response.StatusCode != 200 {
		p.logger.Debug("received bad status", zap.Int("status_code", response.StatusCode))
		return
	}

	if _, err := io.Copy(utils.HttpAutoFlusher(w), response.Body); err != nil {
		p.logger.Debug("Request cancelled", zap.Error(err))
		return
	}

	for key, values := range response.Trailer {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

func writeGrpcStatus(w http.ResponseWriter, code codes.Code, msg string) {
	w.Header().Set("Content-Type", "application/grpc")
	w.WriteHeader(200)
	w.Header().Add("Grpc-Status", strconv.Itoa(int(code)))
	w.Header().Add("Grpc-Message", msg)
}

func (server *upstreamServer) dialOptions() []grpc.DialOption {
	if server.plaintext {
		return []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}

	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: server.insecureSkipVerify})),
	}
}
