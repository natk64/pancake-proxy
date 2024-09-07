package providers

import (
	"context"
	"time"

	"github.com/natk64/pancake-proxy/proxy"
)

type Static struct {
	// Servers defines the servers that this provider will provide.
	Servers []proxy.UpstreamConfig

	// ServiceUpdateInterval specifies how often the services are updated.
	// The default value is 30s.
	ServiceUpdateInterval time.Duration

	// Name can be used to override the provider name.
	// The default is 'static'
	Name string
}

// Run starts the provider and registers the defined servers with the proxy.
// It then continues to update the provided services in the interval specified in the provider.
//
// This function will not return until the context is cancelled.
func (prov Static) Run(ctx context.Context, target *proxy.Proxy) error {
	if prov.Name == "" {
		prov.Name = "static"
	}

	if prov.ServiceUpdateInterval == 0 {
		prov.ServiceUpdateInterval = time.Second * 30
	}

	target.ReplaceServers(prov.Name, prov.Servers)
	target.UpdateServices(prov.Name)

	ticker := time.NewTicker(prov.ServiceUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			target.UpdateServices(prov.Name)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
