package proxy

import (
	"context"
	"sync/atomic"
	"time"

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

	srv.logger.Debug("Creating new reflection client")
	conn, err := grpc.NewClient(srv.config.Address, srv.dialOptions()...)
	if err != nil {
		return nil, err
	}

	client := reflection.NewClient(conn)
	srv.reflectionClient = client
	return client, nil
}

func (srv *upstreamServer) stopWatchingServices() {
	if srv.stopServiceWatcher != nil {
		srv.stopServiceWatcher()
	}
}

func (srv *upstreamServer) watchServices(ctx context.Context, proxy *Proxy) error {
	srv.logger.Debug("Service watcher started")

	ctx, cancel := context.WithCancel(ctx)
	srv.stopServiceWatcher = cancel
	defer func() {
		cancel()
		srv.stopServiceWatcher = nil
		srv.logger.Debug("Service watcher stopped")
	}()

	client, err := srv.reflectClient()
	if err != nil {
		return err
	}

	for {
		var info serviceInfoResult
		var err error

		for {
			info, err = srv.getServiceInfo()
			if err != nil {
				srv.logger.Error("Failed to get service info", zap.Error(err))
				time.Sleep(time.Second * 10)
				continue
			}
			break
		}

		proxy.replaceServices(srv, info)

		select {
		case <-client.Disconnected():
			srv.logger.Debug("Lost connection to server")
			time.Sleep(time.Second * 10)
			srv.logger.Info("Refreshing service info")
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Proxy) replaceServices(targetServer *upstreamServer, info serviceInfoResult) {
	p.logger.Debug("Replacing services", zap.String("target_host", targetServer.config.Address), zap.Strings("services", info.services))

	p.servicesMutex.Lock()
	defer p.servicesMutex.Unlock()

	for _, service := range p.services {
		var filtered []*upstreamServer
		for _, server := range service.servers {
			if server != targetServer {
				filtered = append(filtered, server)
			}
		}
		service.servers = filtered
	}

	for _, serviceName := range info.services {
		service := p.services[serviceName]
		if service == nil {
			service = &upstreamService{}
			p.services[serviceName] = service
		}

		if err := p.reflectionResolver.RegisterFiles(info.fileDescriptors); err != nil {
			p.logger.Error("Failed to register proto files for server", zap.Error(err))
		}

		service.servers = append(service.servers, targetServer)
	}
}

type serviceInfoResult struct {
	services        []string
	fileDescriptors []protoreflect.FileDescriptor
}

func (srv *upstreamServer) getServiceInfo() (serviceInfoResult, error) {
	client, err := srv.reflectClient()
	if err != nil {
		return serviceInfoResult{}, err
	}

	services, err := client.ListServices()
	if err != nil {
		return serviceInfoResult{}, err
	}

	var fds []protoreflect.FileDescriptor
	for _, service := range services {
		allFiles, err := client.AllFilesForSymbol(service)
		if err != nil {
			srv.logger.Error("Failed to resolve service", zap.String("service_name", service), zap.Error(err))
			continue
		}

		fds = append(fds, allFiles...)
	}

	return serviceInfoResult{
		services:        services,
		fileDescriptors: fds,
	}, nil
}
