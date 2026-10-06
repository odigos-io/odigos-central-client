# Odigos Central Client

A Go client for the [Odigos Central](https://docs.odigos.io/central/overview) GraphQL API.
It signs in to a Central server, performs a version handshake, and exposes typed operations
for the connected clusters (compute platforms), their namespaces, and their sources.

This module is the backend of the Odigos Central Terraform provider, and can be used
directly by any Go program that needs to automate Central.

## Installation

```bash
go get github.com/odigos-io/odigos-central-client
```

Requires Go 1.24 or newer. The module depends only on the Go standard library.

## Usage

### Creating a client

`central.NewClient` authenticates with a username and password through the `SignIn`
mutation, checks that the server version is supported, and returns a `*central.CentralClient`.
The access token is attached to every subsequent request automatically.

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/odigos-io/odigos-central-client/central"
)

func main() {
    ctx := context.Background()

    client, err := central.NewClient(ctx, central.ClientConfig{
        Hostname: "central.example.com:8081",
        Username: "user@example.com",
        Password: "your-password",
    })
    if err != nil {
        log.Fatal(err)
    }

    platforms, err := client.GetComputePlatforms(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, p := range platforms {
        fmt.Printf("%s (%s) odigos %s\n", p.Name, p.ID, p.OdigosVersion)
    }
}
```

### Client configuration

`central.ClientConfig` fields:

| Field | Description |
|-------|-------------|
| `Hostname` | Central server host and optional port, for example `central.example.com:8081`. |
| `Username` | Email of a Central user. |
| `Password` | Password for that user. |
| `HTTPClient` | Optional `*http.Client`. When nil, a client with a 30 second timeout (`central.DefaultHTTPTimeout`) is used. |
| `Insecure` | Use plain `http://` instead of `https://`. Off by default. |
| `InsecureSkipVerify` | Keep HTTPS but skip certificate and hostname verification. Only for a Central server with a self-signed certificate. Ignored when `Insecure` is set. |

### Logging

`NewClient` accepts logger options. By default a `slog` logger writing to stderr is used;
its level comes from the `ODIGOS_LOG` environment variable (`DEBUG`, `INFO`, `WARN`, or
`ERROR`, default `INFO`).

```go
import "github.com/odigos-io/odigos-central-client/logger"

// myLogger must implement logger.CustomLogger.
client, err := central.NewClient(ctx, cfg, logger.WithLogger(myLogger))
```

### Central-scoped operations (`*central.CentralClient`)

| Method | Description |
|--------|-------------|
| `GetComputePlatforms(ctx)` | Lists every cluster connected to Central. Successful results are cached. |
| `ResetPlatforms()` | Clears the cached cluster list. |
| `FindPlatformByName(ctx, name)` | Resolves a cluster name to its `types.ComputePlatform`. |
| `Proxy(ctx, proxyID)` | Returns a `*central.ProxyClient` for one cluster, by compute platform ID. |
| `ProxyByName(ctx, name)` | Same as `Proxy`, keyed by cluster name. |
| `Version()` | The Central server version detected during the handshake. |

### Cluster-scoped operations (`*central.ProxyClient`)

Cluster operations go through Central's `remoteFetch` proxy. The client picks the GraphQL
document that matches the cluster's Odigos version, so one `CentralClient` can drive
clusters running different versions.

| Method | Description |
|--------|-------------|
| `GetSources(ctx)` | Lists every source on the cluster. |
| `GetSource(ctx, types.SourceID{Kind, Name, Namespace})` | Returns one source with its conditions and containers. |
| `GetNamespaces(ctx)` | Lists the cluster's namespaces. |
| `GetNamespace(ctx, name)` | Returns one namespace with its sources. |
| `PersistSources(ctx, []types.SourceInput)` | Selects or deselects sources. Returns the mutation's success flag. |
| `PersistNamespaceSources(ctx, []types.NamespaceInput)` | Selects or deselects whole namespaces. |
| `ID()`, `Name()`, `Version()`, `Platform()` | Identity and version of the cluster behind the proxy. |

### Example: list sources on every cluster

```go
platforms, err := client.GetComputePlatforms(ctx)
if err != nil {
    log.Fatal(err)
}

for _, p := range platforms {
    proxy, err := client.Proxy(ctx, p.ID)
    if err != nil {
        // Clusters running an unsupported Odigos version are reported per cluster.
        if central.IsUnsupported(err) {
            log.Printf("skipping %s: %v", p.Name, err)
            continue
        }
        log.Fatal(err)
    }

    sources, err := proxy.GetSources(ctx)
    if err != nil {
        log.Printf("GetSources(%s): %v", p.Name, err)
        continue
    }
    for _, s := range sources {
        fmt.Printf("%s %s/%s selected=%t\n", s.Kind, s.Namespace, s.Name, s.Selected)
    }
}
```

### Example: select a source

```go
import "github.com/odigos-io/odigos-central-client/types"

proxy, err := client.ProxyByName(ctx, "prod-us-east")
if err != nil {
    log.Fatal(err)
}

ok, err := proxy.PersistSources(ctx, []types.SourceInput{{
    Namespace:         "default",
    Name:              "my-deployment",
    Kind:              "Deployment",
    Selected:          true,
    CurrentStreamName: "default",
}})
if err != nil {
    log.Fatal(err)
}
fmt.Println("persisted:", ok)
```

### Errors

`NewClient` and the proxy methods return typed errors you can inspect with `errors.As`:

| Error | Meaning |
|-------|---------|
| `*central.UnsupportedCentralVersionError` | The server is older than `central.MinCentralVersion`. |
| `*central.UnsupportedProxyVersionError` | The cluster's Odigos version is older than `central.MinProxyVersion`. |
| `*central.UnsupportedProxyPlatformError` | The cluster's platform type is not recognised. |
| `*central.GraphQLError` | The server returned GraphQL errors. `Messages()` lists them. |

`central.IsUnsupported(err)` reports whether `err` is any of the unsupported-version errors.

## Packages

| Package | Contents |
|---------|----------|
| `central` | `CentralClient`, `ProxyClient`, `ClientConfig`, and the error types. |
| `types` | Result and input types such as `ComputePlatform`, `Source`, `SourceInput`, `K8sActualNamespace`, and `NamespaceInput`. |
| `operations` | Generated GraphQL documents, one variant per supported Odigos version. |
| `logger` | `CustomLogger` interface and the default `slog` logger. |
| `platform` | Cluster platform types. |
| `version` | Minimal `vMAJOR.MINOR` version parsing and comparison. |

## Regenerating the GraphQL operations

The files under `operations/` are generated by `tools/gen-graphql` from the Odigos Central
UI sources and should not be edited by hand. They are regenerated automatically when the
Central API changes.

## License

[Apache License 2.0](LICENSE)
