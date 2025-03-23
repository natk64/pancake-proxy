package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"slices"

	"github.com/natk64/pancake-proxy/reflection"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type UpstreamConfig struct {
	Address            string `mapstructure:"address"`
	Plaintext          bool   `mapstructure:"plaintext"`
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
}

// upstreamServer should only be created using [newUpstream]
type upstreamServer struct {
	config   UpstreamConfig
	provider string

	stopServiceWatcher func()
	reflectionClient   *reflection.ReflectionClient
	httpClient         *http.Client
	logger             *zap.Logger
}

func newUpstream(provider string, config UpstreamConfig, logger *zap.Logger) *upstreamServer {
	var transport http.RoundTripper
	if config.Plaintext {
		transport = &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		}
	} else {
		transport = &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: config.InsecureSkipVerify,
			},
		}
	}

	return &upstreamServer{
		config:   config,
		provider: provider,
		logger:   logger,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (server *upstreamServer) dialOptions() []grpc.DialOption {
	if server.config.Plaintext {
		return []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}

	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: server.config.InsecureSkipVerify})),
	}
}

// cleanupServer should be run when a server is removed from the server list.
func (p *Proxy) cleanupServer(server *upstreamServer) {
	server.logger.Debug("Cleaning up server")
	server.stopWatchingServices()
	p.servicesMutex.Lock()
	defer p.servicesMutex.Unlock()

	for _, service := range p.services {
		if serverIndex := slices.Index(service.servers, server); serverIndex != -1 {
			service.servers[serverIndex] = service.servers[len(service.servers)-1]
			service.servers = service.servers[:len(service.servers)-1]
		}
	}
}

func (p *Proxy) ReplaceServers(provider string, newConfigs []UpstreamConfig) {
	p.logger.Info("Replacing servers of provider", zap.String("provider", provider), zap.Int("count", len(newConfigs)))

	shouldBuildConfigs := make(map[UpstreamConfig]bool)
	for _, server := range newConfigs {
		shouldBuildConfigs[server] = true
	}

	p.serverMutex.Lock()
	defer p.serverMutex.Unlock()

	var newServers []*upstreamServer
	oldServers := p.servers[provider]

	for _, server := range oldServers {
		if _, ok := shouldBuildConfigs[server.config]; ok {
			newServers = append(newServers, server)
			shouldBuildConfigs[server.config] = false
		} else {
			p.cleanupServer(server)
		}
	}

	for config, shouldBuild := range shouldBuildConfigs {
		if shouldBuild {
			server := newUpstream(provider, config, p.logger.Named("upstream").With(zap.String("upstream_host", config.Address)))
			go server.watchServices(context.Background(), p)
			newServers = append(newServers, server)
			server.logger.Debug("Adding server to new server list")
		}
	}

	p.servers[provider] = newServers
}
