# Odigos Central Client

A Go library for interacting with the Odigos Central GraphQL API.

## Installation

```bash
go get github.com/odigos-io/odigos-central-client
```

## Usage

### Creating a Client

The client authenticates with username and password (SignIn); an access token is obtained automatically.

```go
package main

import (
    "context"
    "github.com/odigos-io/odigos-central-client"
)

func main() {
    ctx := context.Background()
    client, err := odigos.NewClient(ctx, odigos.ClientConfig{
        Hostname: "your-odigos-instance.com",
        Username: "your-email@example.com",
        Password: "your-password",
        Insecure: false, // use true for http:// instead of https://
    })
    if err != nil {
        // handle sign-in error
    }
    platforms, err := client.GetComputePlatforms(ctx)
    // ...
}
```

### Client Configuration

`ClientConfig` supports:

- **Hostname** – Odigos Central host (e.g. `central.odigos.io`)
- **Username** – Email for sign-in
- **Password** – Password for sign-in
- **Insecure** – If true, use `http://` instead of `https://`
- **HTTPClient** – Optional custom `*http.Client` (defaults to `http.DefaultClient`)

`NewClient` returns `*OdigosClient`. Optional logger options can be passed as variadic arguments:

```go
import "github.com/odigos-io/odigos-central-client/logger"

// Use a custom logger (must implement logger.CustomLogger).
client, err := odigos.NewClient(ctx, config, logger.WithLogger(myLogger))
```

If you omit options, a default `logger.NewSlogLogger` is used. Its level is controlled by the `ODIGOS_LOG` environment variable (`DEBUG`, `INFO`, `WARN`, or `ERROR`; default `INFO`).

### API Methods (`*OdigosClient`)

All methods take `context.Context` as the first argument. Methods that target a specific cluster use a `proxyID` (compute platform ID).

| Method | Description |
|--------|-------------|
| `GetComputePlatforms(ctx)` | Returns all compute platforms (clusters). No `proxyID`. |
| `GetNamespaces(ctx, proxyID)` | Lists Kubernetes namespaces for a cluster. |
| `GetNamespace(ctx, proxyID, namespaceName)` | Returns details for one namespace. |
| `GetSources(ctx, proxyID)` | Lists observability sources for a cluster. |
| `GetSource(ctx, proxyID, sourceID)` | Returns one source by ID (kind, name, namespace). |
| `PersistSources(ctx, proxyID, sources)` | Persists source selection (e.g. data stream names). |
| `PersistNamespaceSources(ctx, proxyID, namespaces)` | Persists namespace-level source selection. |

### Logger package (`logger`)

| Function | Description |
|----------|-------------|
| `NewSlogLogger()` | Default stderr `slog` logger used when no `WithLogger` is passed. |
| `WithLogger(CustomLogger)` | Returns a `LoggingOption` for `NewClient`. |

### Advanced: `UnmarshalRemoteFetch`

Cluster-scoped GraphQL calls go through Central’s `remoteFetch` proxy. If you have a raw GraphQL response body (bytes) from that flow, you can parse the nested payload with the generic helper:

| Function | Description |
|----------|-------------|
| `UnmarshalRemoteFetch[T](graphqlResp []byte) (*T, error)` | Unmarshals the outer response and inner remote GraphQL JSON into `T`. Useful for tests or custom clients; `*OdigosClient` already uses this internally. |

### Example: List platforms and then sources

```go
ctx := context.Background()
client, err := odigos.NewClient(ctx, odigos.ClientConfig{
    Hostname: "central.odigos.io",
    Username: "user@example.com",
    Password: "secret",
})
if err != nil {
    log.Fatal(err)
}

platforms, err := client.GetComputePlatforms(ctx)
if err != nil {
    log.Fatal(err)
}

for _, p := range platforms {
    sources, err := client.GetSources(ctx, p.ID)
    if err != nil {
        log.Printf("GetSources(%s): %v", p.ID, err)
        continue
    }
    for _, s := range sources {
        fmt.Printf("%s/%s %s\n", s.Namespace, s.Name, s.Kind)
    }
}
```

### Example: Persist source selection

```go
sources := []odigos.SourceInput{
    {
        Namespace:         "default",
        Name:              "my-deployment",
        Kind:              "Deployment",
        Selected:          true,
        CurrentStreamName: "default",
    },
}
ok, err := client.PersistSources(ctx, proxyID, sources)
if err != nil {
    log.Fatal(err)
}
// ok indicates success
```

### Types

Input and result types used by the API (e.g. `ComputePlatform`, `Source`, `SourceInput`, `SourceId`, `K8sActualNamespace`, `K8sActualNamespaceDetail`, `NamespaceInput`) are defined in the package. See the package documentation or `responseStructs.go` for field details.

## Dependencies

This library uses the Go standard library for HTTP and JSON.

No third-party GraphQL client is required; requests are implemented with `net/http` and `encoding/json`.

## License

[Your License Here]
