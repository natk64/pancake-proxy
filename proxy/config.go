package proxy

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/natk64/pancake-proxy/reflection"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type UpstreamConfig struct {
	Address            string `mapstructure:"address"`
	Plaintext          bool   `mapstructure:"plaintext"`
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
}

type ServerConfig struct {
	Upstreams             []UpstreamConfig `mapstructure:"servers"`
	ServiceUpdateInterval time.Duration    `mapstructure:"serviceUpdateInterval"`

	// DisableReflection will not expose the reflection service
	DisableReflection bool `mapstructure:"disableReflection"`

	Logger *zap.Logger
}

func NewServer(config ServerConfig) *proxy {
	p := &proxy{
		reflectionResolver:       &reflection.SimpleResolver{},
		services:                 make(map[string]*upstreamService),
		servicesMutex:            &sync.RWMutex{},
		serviceUpdateInterval:    config.ServiceUpdateInterval,
		upstreams:                upstreamConfig(config.Upstreams),
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

func upstreamConfig(upstreams []UpstreamConfig) []*upstreamServer {
	servers := make([]*upstreamServer, len(upstreams))
	for i, config := range upstreams {
		servers[i] = makeServer(&config)
	}
	return servers
}

func makeServer(config *UpstreamConfig) *upstreamServer {
	return &upstreamServer{
		host:               config.Address,
		plaintext:          config.Plaintext,
		insecureSkipVerify: config.InsecureSkipVerify,
		httpClient: &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: config.InsecureSkipVerify,
				},
			},
		},
	}
}
