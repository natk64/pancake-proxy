package proxy

import (
	"cmp"
	_ "embed"
	"html/template"
	"net/http"
	"slices"

	"go.uber.org/zap"
)

//go:embed dashboard/index.html
var dashboardTemplateContent string
var dashboardTemplate = template.Must(template.New("index.html").Parse(dashboardTemplateContent))

type DashboardServerInfo struct {
	Config   UpstreamConfig
	Provider string
	Services []*DashboardServiceInfo
}

type DashboardServiceInfo struct {
	Name    string
	Servers []*DashboardServerInfo
}

type DashboardContext struct {
	ReflectionDisabled bool
	Services           []*DashboardServiceInfo
	Servers            []*DashboardServerInfo
	UnknownServer      *DashboardServerInfo
}

var unknownServer = &DashboardServerInfo{Config: UpstreamConfig{Address: "INVALID SERVER"}}

// DashboardContext creates the struct that is used as the template data for the dashboard.
func (p *Proxy) DashboardContext() DashboardContext {
	p.serverMutex.RLock()
	defer p.serverMutex.RUnlock()

	serverMap := make(map[*upstreamServer]*DashboardServerInfo)
	var serverList []*DashboardServerInfo

	for _, servers := range p.servers {
		for _, server := range servers {
			info := &DashboardServerInfo{
				Config:   server.config,
				Provider: server.provider,
			}

			serverMap[server] = info
			serverList = append(serverList, info)
		}
	}

	serviceList := make([]*DashboardServiceInfo, 0, len(p.services))

	p.servicesMutex.RLock()
	defer p.servicesMutex.RUnlock()

	servicesSorted := sortedKVs(p.services)

	for _, kv := range servicesSorted {
		service := kv.value
		serviceName := kv.key
		serviceInfo := &DashboardServiceInfo{
			Name:    serviceName,
			Servers: make([]*DashboardServerInfo, len(service.servers)),
		}

		for i, server := range service.servers {
			serverInfo := serverMap[server]
			if serverInfo == nil {
				// The service config lists a server that is not in the server list.
				// Hopefully this never happens, but if it does, we use unknownServer as a placeholder.
				serviceInfo.Servers[i] = unknownServer
				continue
			}

			serviceInfo.Servers[i] = serverInfo
			serverInfo.Services = append(serverInfo.Services, serviceInfo)
		}

		serviceList = append(serviceList, serviceInfo)
	}

	return DashboardContext{
		ReflectionDisabled: p.disableReflectionService,
		Services:           serviceList,
		Servers:            serverList,
		UnknownServer:      unknownServer,
	}
}

// DashboardHandler serves the dashboard index.html file.
func (p *Proxy) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	if err := dashboardTemplate.Execute(w, p.DashboardContext()); err != nil {
		p.logger.Error("Failed to execute dashboard template", zap.Error(err))
		w.WriteHeader(500)
	}
}

type kv[K, V any] struct {
	key   K
	value V
}

func sortedKVs[K cmp.Ordered, V any](m map[K]V) []kv[K, V] {
	kvs := make([]kv[K, V], 0, len(m))
	for key, value := range m {
		kvs = append(kvs, kv[K, V]{key: key, value: value})
	}
	slices.SortFunc(kvs, func(a, b kv[K, V]) int {
		return cmp.Compare(a.key, b.key)
	})
	return kvs
}
