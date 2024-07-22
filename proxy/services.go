package proxy

import (
	"sync"
	"sync/atomic"

	"github.com/natk64/pancake-proxy/reflection"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type upstreamService struct {
	servers []*upstreamServer
	next    atomic.Uint32
}

// reflectClient returns the grpc reflection client for the server.
// If no client is connected, a new one is created.
func (srv *upstreamServer) reflectClient() (*reflection.ReflectionClient, error) {
	if srv.reflectionClient != nil {
		return srv.reflectionClient, nil
	}

	conn, err := grpc.NewClient(srv.config.Address, srv.dialOptions()...)
	if err != nil {
		return nil, err
	}

	client := reflection.NewClient(conn)
	srv.reflectionClient = client
	return client, nil
}

// updateServices checks all upstream server of a specific provider and resolves the services they expose.
func (p *Proxy) UpdateServices(provider string) {
	type result struct {
		server          *upstreamServer
		services        []string
		fileDescriptors []protoreflect.FileDescriptor
	}

	p.serverMutex.RLock()
	upstreams := p.servers[provider]
	p.serverMutex.RUnlock()
	results := make([]result, len(upstreams))
	wg := sync.WaitGroup{}
	wg.Add(len(upstreams))

	logger := p.logger.With(zap.String("provider", provider))
	logger.Debug("Updating upstream servers", zap.Int("count", len(upstreams)))

	for i, server := range upstreams {
		go func(i int, server *upstreamServer) {
			defer wg.Done()

			logger := logger.With(zap.String("target_host", server.config.Address))
			client, err := server.reflectClient()
			if err != nil {
				logger.Error("Failed to create reflection client", zap.Error(err))
				return
			}

			services, err := client.ListServices()
			if err != nil {
				logger.Error("Failed to query services", zap.Error(err))
				return
			}

			var fds []protoreflect.FileDescriptor
			for _, service := range services {
				allFiles, err := client.AllFilesForSymbol(service)
				if err != nil {
					logger.Error("Failed to resolve service", zap.String("service_name", service))
					continue
				}

				fds = append(fds, allFiles...)
			}

			results[i] = result{
				server:          server,
				services:        services,
				fileDescriptors: fds,
			}
		}(i, server)
	}

	wg.Wait()

	p.servicesMutex.Lock()
	defer p.servicesMutex.Unlock()

	for _, service := range p.services {
		var filtered []*upstreamServer
		for _, server := range service.servers {
			if server.provider != provider {
				filtered = append(filtered, server)
			}
		}
		service.servers = filtered
	}

	for _, result := range results {
		for _, serviceName := range result.services {
			service := p.services[serviceName]
			if service == nil {
				service = &upstreamService{}
				p.services[serviceName] = service
			}

			if err := p.reflectionResolver.RegisterFiles(result.fileDescriptors); err != nil {
				p.logger.Error("Failed to register proto files for server", zap.Error(err), zap.String("target_host", result.server.config.Address))
			}

			service.servers = append(service.servers, result.server)
		}
	}
}
