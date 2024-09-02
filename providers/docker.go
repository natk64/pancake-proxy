package providers

import (
	"context"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/natk64/pancake-proxy/proxy"
	"go.uber.org/zap"
)

type Docker struct {
	// ExposeMode controls how the provider decides which services to expose.
	// See the individual modes for explanations.
	// The default mode is [ExposeManual].
	ExposeMode ExposeMode

	// Label specifies the prefix of the labels used to configure servers.
	// e.g. [pancake].enabled=true
	// The default is 'pancake'.
	Label string

	// DockerHost specifies the host of the Docker socket.
	// The default is [client.DefaultDockerHost]
	DockerHost string

	// DefaultNetwork specifies the default network to use for connections to containers.
	// Different networks are tried in the following order:
	//	1. The value of the pancake.network label
	//	2. The default network specified here
	//	3. The loopback address if the container's network mode is 'host'
	//	4. Fail and ignore container
	DefaultNetwork string

	// ExposedProject specifies the which Docker compose projects to expose when using [ExposeMode] == [ExposeProjects].
	ExposedProjects []string

	// Name can be used to override the provider name, default 'docker'.
	Name string

	// The logger to use.
	// The default is the global logger.
	Logger *zap.Logger

	client *client.Client
	target *proxy.Proxy
	labels knownLabels
}

type ExposeMode string

const (
	// ExposeAll attempts to expose all Docker services over the proxy.
	// Note that this will not work if the services and proxy don't share a network.
	ExposeAll ExposeMode = "all"

	// ExposeManual only exposes services which have the pancake.enable label set to 'true'.
	ExposeManual ExposeMode = "manual"

	// ExposeSameProject exposes all services which are defined in the same compose project as the proxy.
	// Only works on linux.
	ExposeSameProject ExposeMode = "same_project"

	// ExposeProjects exposes all services which are defined in compose projects specified in [Docker.ExposedProjects].
	ExposeProjects ExposeMode = "projects"
)

const composeProjectLabel = "com.docker.compose.project"

type knownLabels struct {
	enable     string
	plaintext  string
	skipVerify string
	port       string
	network    string
}

// Run starts the provider.
// It will block until an error occurs or the context is cancelled.
func (prov Docker) Run(ctx context.Context, target *proxy.Proxy) error {
	mode, err := checkExposeMode(prov.ExposeMode)
	if err != nil {
		return err
	}
	if prov.DockerHost == "" {
		prov.DockerHost = client.DefaultDockerHost
	}
	if prov.Label == "" {
		prov.Label = "pancake"
	}
	if prov.Logger == nil {
		prov.Logger = zap.L().Named("docker_provider")
	}
	if prov.Name == "" {
		prov.Name = "docker"
	}

	prov.labels = knownLabels{
		enable:     fmt.Sprintf("%s.enable", prov.Label),
		plaintext:  fmt.Sprintf("%s.plaintext", prov.Label),
		skipVerify: fmt.Sprintf("%s.skip_verify", prov.Label),
		port:       fmt.Sprintf("%s.port", prov.Label),
		network:    fmt.Sprintf("%s.network", prov.Label),
	}

	prov.ExposeMode = mode
	prov.target = target
	prov.client, err = client.NewClientWithOpts(client.WithHost(prov.DockerHost), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	containers, err := prov.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}

	if err := prov.handleContainerList(containers); err != nil {
		prov.Logger.Error("An error occurred while loading the initial container list", zap.Error(err))
		return err
	}

	receiveEvents := func() error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		messages, errs := prov.client.Events(ctx, events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("type", "container"),
			),
		})

		for {
			select {
			case msg := <-messages:
				if err := prov.handleEvent(ctx, msg); err != nil {
					return err
				}
			case err := <-errs:
				return err
			}
		}
	}

	for {
		if err := receiveEvents(); err != nil {
			prov.Logger.Error("An error occurred while listening for events", zap.Error(err))
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		time.Sleep(time.Second * 10)
		prov.Logger.Info("Restarting event listener")
	}
}

func (prov *Docker) handleEvent(ctx context.Context, event events.Message) error {
	if event.Type != events.ContainerEventType {
		return nil
	}

	if event.Action != events.ActionStart && event.Action != events.ActionStop && event.Action != events.ActionDie {
		return nil
	}

	containers, err := prov.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}

	return prov.handleContainerList(containers)
}

func (prov *Docker) handleContainerList(containers []types.Container) error {
	var servers []proxy.UpstreamConfig

	for _, container := range containers {
		if prov.ExposeMode == ExposeManual && container.Labels[prov.labels.enable] != "true" {
			continue
		}

		if prov.ExposeMode == ExposeProjects && !slices.Contains(prov.ExposedProjects, container.Labels[composeProjectLabel]) {
			continue
		}

		if prov.ExposeMode == ExposeSameProject {
			self, err := getOwnContainer(containers)
			if err != nil {
				return fmt.Errorf("failed to find own container, %w", err)
			}

			if self.Labels[composeProjectLabel] == "" || self.Labels[composeProjectLabel] != container.Labels[composeProjectLabel] {
				continue
			}
		}

		config, err := prov.configFromContainer(&container)
		if err != nil {
			prov.Logger.Error("An error occurred while trying to build the server config",
				zap.Error(err),
				zap.String("container", containerName(&container)))
		} else {
			servers = append(servers, config)
		}
	}

	prov.target.ReplaceServers(prov.Name, servers)
	time.AfterFunc(time.Second*10, func() { prov.target.UpdateServices(prov.Name) })
	return nil
}

func (prov *Docker) configFromContainer(container *types.Container) (proxy.UpstreamConfig, error) {
	ip, err := prov.getIpAddress(container)
	if err != nil {
		return proxy.UpstreamConfig{}, fmt.Errorf("failed to get ip, %w", err)
	}
	port, err := prov.getPort(container)
	if err != nil {
		return proxy.UpstreamConfig{}, fmt.Errorf("failed to get port, %w", err)
	}

	return proxy.UpstreamConfig{
		Plaintext:          container.Labels[prov.labels.plaintext] == "true",
		InsecureSkipVerify: container.Labels[prov.labels.skipVerify] == "true",
		Address:            net.JoinHostPort(ip, port),
	}, nil
}

func (prov *Docker) getPort(container *types.Container) (string, error) {
	if port := container.Labels[prov.labels.port]; port != "" {
		return port, nil
	}

	if len(container.Ports) != 1 {
		return "", fmt.Errorf("cannot determine port, found %d expected 1", len(container.Ports))
	}

	return strconv.Itoa(int(container.Ports[0].PrivatePort)), nil
}

func (prov *Docker) getIpAddress(container *types.Container) (string, error) {
	network := container.Labels[prov.labels.network]
	if network == "" {
		network = prov.DefaultNetwork
	}

	inspectResult, err := prov.client.ContainerInspect(context.Background(), container.ID)
	if err != nil {
		return "", err
	}

	if inspectResult.HostConfig.NetworkMode.IsHost() {
		return "127.0.0.1", nil
	}

	if network == "" {
		return "", fmt.Errorf("no network specified and the network mode is not 'host'")
	}

	networkSettings := container.NetworkSettings.Networks[network]
	if networkSettings == nil {
		return "", fmt.Errorf("container is not in the specified network '%s'", network)
	}

	if networkSettings.IPAddress != "" {
		return networkSettings.IPAddress, nil
	}
	return networkSettings.GlobalIPv6Address, nil
}

func checkExposeMode(mode ExposeMode) (ExposeMode, error) {
	valid := []ExposeMode{ExposeAll, ExposeManual, ExposeSameProject}
	if slices.Contains(valid, mode) {
		return mode, nil
	}

	if mode == "" {
		return ExposeManual, nil
	}

	return mode, fmt.Errorf("invalid expose mode '%s'", mode)
}

func containerName(container *types.Container) string {
	if len(container.Names) == 0 {
		return container.ID
	}
	return container.Names[0]
}

func getOwnContainer(containers []types.Container) (*types.Container, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}

	split := strings.SplitAfterN(string(data), "/docker/containers/", 2)
	if len(split) != 2 {
		return nil, fmt.Errorf("no container id found in /proc/self/mountinfo")
	}
	containerId := split[1]
	end := strings.Index(containerId, "/")
	if end != -1 {
		containerId = containerId[:end]
	}

	for _, container := range containers {
		if container.ID == containerId {
			return &container, nil
		}
	}

	return nil, fmt.Errorf("no container found with id '%s'", containerId)
}
