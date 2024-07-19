package proxy

import (
	"context"
	"sync"

	"github.com/jhump/protoreflect/grpcreflect"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// reflectClient returns the grpc reflection client for the server.
// If no client is connected, a new one is created.
func (srv *serverInfo) reflectClient() (*grpcreflect.Client, error) {
	if srv.reflectionClient != nil {
		return srv.reflectionClient, nil
	}

	conn, err := grpc.NewClient(srv.host, srv.dialOptions()...)
	if err != nil {
		return nil, err
	}

	client := grpcreflect.NewClientAuto(context.Background(), conn)
	srv.reflectionClient = client
	return client, nil
}

// updateServices updates the list of upstream servers and resolves the services they provide.
func (p *proxy) updateServices() {
	type result struct {
		server          *serverInfo
		services        []string
		fileDescriptors []protoreflect.FileDescriptor
	}

	var results []result

	upstreams := p.upstreams
	wg := sync.WaitGroup{}
	wg.Add(len(upstreams))

	p.logger.Debug("Updating upstream servers", zap.Int("count", len(upstreams)))

	for _, server := range upstreams {
		go func(server *serverInfo) {
			defer wg.Done()

			logger := p.logger.With(zap.String("target_host", server.host))
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
				fd, err := client.FileContainingSymbol(service)
				if err != nil {
					logger.Error("Failed to resolve service", zap.String("service_name", service))
					continue
				}

				fds = append(fds, fd.UnwrapFile())
			}

			results = append(results, result{
				server:          server,
				services:        services,
				fileDescriptors: fds,
			})
		}(server)
	}

	wg.Wait()

	p.servicesMutex.Lock()
	defer p.servicesMutex.Unlock()

	for k := range p.services {
		delete(p.services, k)
	}

	for _, result := range results {
		for _, serviceName := range result.services {
			service := p.services[serviceName]
			if service == nil {
				service = &upstreamService{}
				p.services[serviceName] = service
			}

			if err := p.reflectionResolver.RegisterFiles(result.fileDescriptors); err != nil {
				p.logger.Error("Failed to register proto files for server", zap.Error(err), zap.String("target_host", result.server.host))
			}

			service.servers = append(service.servers, result.server)
		}

	}
}
