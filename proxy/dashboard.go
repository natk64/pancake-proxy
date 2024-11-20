package proxy

import (
	_ "embed"
	"html/template"
	"net/http"

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

var unknownServer = &DashboardServerInfo{}

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
	for serviceName, service := range p.services {
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

func (p *Proxy) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	if err := dashboardTemplate.Execute(w, p.DashboardContext()); err != nil {
		p.logger.Error("Failed to execute dashboard template", zap.Error(err))
		w.WriteHeader(500)
	}
}
