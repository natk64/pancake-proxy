package proxy

import (
	"sync"
	"time"
)

type UpstreamConfig struct {
	Address            string `mapstructure:"address"`
	Plaintext          bool   `mapstructure:"plaintext"`
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
}

type ServerConfig struct {
	ServiceUpdateInterval time.Duration    `mapstructure:"serviceUpdateInterval"`
	Upstreams             []UpstreamConfig `mapstructure:"upstreams"`
}

func NewServer(config ServerConfig) *proxy {
	p := &proxy{
		services:              make(map[string]*upstreamService),
		servicesMutex:         &sync.RWMutex{},
		serviceUpdateInterval: config.ServiceUpdateInterval,
		upstreams:             upstreamConfig(config.Upstreams),
	}

	return p
}

func upstreamConfig(upstreams []UpstreamConfig) []*serverInfo {
	servers := make([]*serverInfo, len(upstreams))
	for i, config := range upstreams {
		servers[i] = &serverInfo{
			host:               config.Address,
			plaintext:          config.Plaintext,
			insecureSkipVerify: config.InsecureSkipVerify,
		}
	}
	return servers
}
