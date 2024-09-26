package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

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

func (p *Proxy) ReplaceServers(provider string, newConfigs []UpstreamConfig) {
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
			server.stopWatchingServices()
		}
	}

	for config, shouldBuild := range shouldBuildConfigs {
		if shouldBuild {
			server := newUpstream(provider, config, p.logger.Named("upstream").With(zap.String("upstream_host", config.Address)))
			go server.watchServices(context.Background(), p)
			newServers = append(newServers, server)
		}
	}

	p.servers[provider] = newServers
}
