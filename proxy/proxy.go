package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/natk64/pancake-proxy/grpcweb"
	"github.com/natk64/pancake-proxy/reflection"
	"github.com/natk64/pancake-proxy/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type ProxyConfig struct {
	// DisableReflection will not expose the reflection service
	DisableReflection bool `mapstructure:"disableReflection"`

	Logger *zap.Logger
}

// Proxy must be created using [NewProxy]
type Proxy struct {
	servers     map[string][]*upstreamServer
	serverMutex *sync.RWMutex

	reflectionResolver *reflection.SimpleResolver

	services      map[string]*upstreamService
	servicesMutex *sync.RWMutex

	internalServer *grpc.Server
	logger         *zap.Logger

	disableReflectionService bool
}

func NewServer(config ProxyConfig) *Proxy {
	p := &Proxy{
		reflectionResolver:       &reflection.SimpleResolver{},
		services:                 make(map[string]*upstreamService),
		servicesMutex:            &sync.RWMutex{},
		servers:                  make(map[string][]*upstreamServer),
		serverMutex:              &sync.RWMutex{},
		internalServer:           grpc.NewServer(),
		logger:                   config.Logger,
		disableReflectionService: config.DisableReflection,
	}

	if p.logger == nil {
		p.logger = zap.NewNop()
	}

	grpc_reflection_v1.RegisterServerReflectionServer(p.internalServer, p)
	grpc_reflection_v1alpha.RegisterServerReflectionServer(p.internalServer, reflection.AlphaConverter{Inner: p})

	return p
}

// ServeHTTP implements the http.Handler interface.
// This method is the entrypoint for all requests into the proxy.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
func (p *Proxy) findServer(serviceName string) (*upstreamServer, bool) {
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
func (p *Proxy) getTargetService(r *http.Request) (name string, ok bool) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	split := strings.SplitN(path, "/", 2)
	if len(split) != 2 {
		return "", false
	}

	return split[0], true
}

// forwardRequest forwards an incoming gRPC request to the specified server.
func (p *Proxy) forwardRequest(req *http.Request, w http.ResponseWriter, server *upstreamServer) {
	req.URL.Host = server.config.Address
	req.Host = server.config.Address
	req.RequestURI = ""

	if server.config.Plaintext {
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
