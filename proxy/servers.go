package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/jhump/protoreflect/grpcreflect"
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

	reflectionClient *grpcreflect.Client
	httpClient       *http.Client
}

func newUpstream(provider string, config UpstreamConfig) *upstreamServer {
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
		}
	}

	for config, shouldBuild := range shouldBuildConfigs {
		if shouldBuild {
			newServers = append(newServers, newUpstream(provider, config))
		}
	}

	p.servers[provider] = newServers
}