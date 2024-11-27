# Pancake

Pancake is a smart gRPC reverse proxy or API gateway, allowing many microservices to be exposed as a single server.

[Docker Hub](https://hub.docker.com/r/natk64/pancake)

## How it works

Pancake automatically discovers gRPC services on different upstream servers using the standard reflection service.
When a gRPC request arrives at Pancake, it automatically forwards that request to an upstream server that implements the requested service.
If multiple servers implement the same service, load balancing is performed as well.

This functionality makes configuration extremely easy and allows Pancake to basically work "out of the box".
A minimal configuration could look something like this:

```yaml
servers:
    # Simply list all servers in your cluster
    # Pancake will figure out how to route requests by itself
    - address: localhost:5000
    - address: localhost:5001
```

## Configuration

Configuration can be done through a config file or environment variables. The config file can located at either /etc/pancake/config.yaml or ./config.yaml.

The documentation will use the yaml names, which can be converted to environment variables like so:

```yaml
bind_address: localhost:5000
docker:
    enabled: true
    label: pancake
```

Equivalent environment variables:

```sh
PANCAKE_BIND_ADDRESS=localhost:5000
PANCAKE_DOCKER_ENABLED=true
PANCAKE_DOCKER_LABEL=pancake
```

Option                  | Type                                        | Default                       | Description
------------------------|---------------------------------------------|-------------------------------|-----------------------------------------------------------------------------------------------------
bind_address            | string                                      | :8080                         | gRPC and gRPC Web entrypoint listener
service_update_interval | duration (5m10s, 3h, etc.)                  | 30s                           | Interval to update static servers
disable_reflection      | bool                                        | false                         | Disables the reflection service
cors.enabled            | bool                                        | false                         | Enable/Disable CORS
cors.allowed_origins    | []string                                    | []                            | Allowed origins for CORS requests
cors.allowed_headers    | []string                                    | [*]                           | Allowed headers for CORS requests
tls.enabled             | bool                                        | true                          | Enable/Disable TLS for incoming gRPC requests.
tls.cert_file           | string                                      | ./server.crt                  | TLS cert file location, only if TLS is enabled
tls.key_file            | string                                      | ./server.key                  | -
pprof.enabled           | bool                                        | false                         | Enable/Disable pprof HTTP server
pprof.bind_address      | string                                      | localhost:6060                | -
dashboard.enabled       | bool                                        | false                         | Enable/Disable the HTML dashboard
dashboard.bind_address  | string                                      | localhost:8081                | -
logger.development      | bool                                        | false                         | Enable debug logs
docker.enabled          | bool                                        | false                         | Enable/Disable the docker provider, more information on this in the [Docker section](#docker) below.
docker.expose           | 'all', 'manual', 'same_project', 'projects' | manual                        | Decision strategy on which services to expose.
docker.label            | string                                      | pancake                       | Override the label prefix used for docker label defined options.
docker.host             | string                                      | unix:///var/run/docker.sock   | The host of the docker socket.
docker.exposed_projects | []string                                    | []                            | The list of projects to expose when docker.expose = 'projects'
docker.network          | string                                      | See [Docker section](#docker) | Which network to use for internal communication with the upstream containers.

## Static server configuration

A static list of servers can be defined in the config.yaml file.

```yaml
servers:
    - address: localhost:5001 # Required, the address of the server
      plaintext: false # Disable TLS, default false (i.e use TLS)
      insecure: false # Disable server certificate verification, default false, no effect if plaintext: true

# Other options
bind_address: :5000
# ...
```

## Docker

This section only applies if your services are running in Docker, Pancake itself doesn't need to run in Docker.
Pancake only needs access to the Docker socket.

Instead of defining a fixed list of servers in the config.yaml, servers can be automatically discovered through Docker.
You can control service discovery through the docker.* options above and through labels on your server containers.

Pancake needs to share a Docker network with your containers to communicate with them (Or use the 'host' network).
The network that pancake will use is determined in the following order:

1. The value of the pancake.network label
2. The default network specified in the docker.network option
3. The loopback address if the container's network mode is 'host'

If no network can be found, the container will be ignored and an error printed.

### Labels

The label prefix 'pancake.' can be changed with docker.label, to allow multiple Pancake instances per Docker installation.

Example docker-compose.yaml

```yaml
services:
    my-service:
        image: my-image
        networks:
            - my-network
        labels:
            - pancake.enable=true
            - pancake.network=my-network
            - pancake.port=5000
```

Label               | Description
--------------------|------------------------------------------------------------------------------------------------------------------------------------------------
pancake.enable      | Set to 'false' to ignore a container. If docker.expose == 'manual', this needs to be explicitly set to 'true' for the container to be included.
pancake.plaintext   | Disable TLS for communication with container, default is 'false'
pancake.skip_verify | Disable server certificate verification for communication with container, default is 'false'
pancake.port        | Which port to use for communication (this is the internal port in your container) (If unspecified, Pancake will try to figure it out by itself)
pancake.network     | Which network to use for communication (See above for what is used when this isn't set).

## Reflection and Healthchecks

Obviously, load balancing the Reflection and Health services would cause issues, but Pancake will also take care of that.

Pancake provides it's own reflection service, which will include all the services on the upstream servers.
For example, if you have two servers, one implementing ServiceA, the other ServiceB, calling ListServices on
Pancake will report ["ServiceA", "ServiceB", ...]

To clients it would look like they're talking to a single server, implementing both services.

## gRPC-Web support

Pancake translates and forwards incoming gRPC-Web requests (Content-Type: grpc-web*) to the upstream servers.
This feature is enabled by default and is usable using the default configuration,
although CORS will need to configured to accept requests from browsers.
