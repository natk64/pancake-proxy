# Pancake

Pancake is a smart gRPC reverse proxy or API gateway, allowing many microservices to be exposed as a single server.

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

## Reflection and Healthchecks

Obviously, load balancing the Reflection and Health services would cause issues, but Pancake will also take care of that.

Pancake provides it's own reflection service, which will include all the services on the upstream servers.
For example, if you have two servers, one implementing ServiceA, the other ServiceB, calling ListServices on
Pancake will report ["ServiceA", "ServiceB", ...]

To clients it would look like they're talking to a single server, implementing both services.
