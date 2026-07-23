package central

import (
	"context"

	"github.com/odigos-io/odigos-central-client/operations"
	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/types"
	"github.com/odigos-io/odigos-central-client/version"
)

// ProxyClient is a CentralClient handle bound to a single compute platform
// (cluster). All cluster-scoped operations on it route through Central's
// remoteFetch proxy and use the right GraphQL document for the proxy's
// version + platform combination.
//
// Obtain one via CentralClient.Proxy or CentralClient.ProxyByName.
type ProxyClient struct {
	tx           remoteFetcher
	id           string
	name         string
	version      version.Version
	platformType platform.Type
}

// ID returns the proxy's compute-platform ID.
func (p *ProxyClient) ID() string { return p.id }

// Name returns the proxy's display name (cluster name).
func (p *ProxyClient) Name() string { return p.name }

// Version returns the proxy's advertised odigosVersion.
func (p *ProxyClient) Version() version.Version { return p.version }

// Platform returns the proxy's platform type (K8s, Vm, ...).
func (p *ProxyClient) Platform() platform.Type { return p.platformType }

// pick is the common entry point for selecting the right GraphQL document.
// Errors from Pick are *operations.UnsupportedPlatformError or
// *operations.UnsupportedVersionError; both are detectable via errors.As.
func (p *ProxyClient) pick(op *operations.Operation) (string, error) {
	return op.Pick(p.platformType, p.version)
}

// GetSources returns every source registered on the cluster.
func (p *ProxyClient) GetSources(ctx context.Context) ([]types.Source, error) {
	q, err := p.pick(&operations.GET_SOURCES)
	if err != nil {
		return nil, err
	}
	var data struct {
		ComputePlatform struct {
			Sources []types.Source `json:"sources"`
		} `json:"computePlatform"`
	}
	if err := p.tx.remoteFetch(ctx, "GetSources", p.id, q, nil, &data); err != nil {
		return nil, err
	}
	return data.ComputePlatform.Sources, nil
}

// GetSource returns the detailed view of a single source identified by its
// (kind, name, namespace) triple.
func (p *ProxyClient) GetSource(ctx context.Context, id types.SourceID) (types.Source, error) {
	q, err := p.pick(&operations.GET_SOURCE)
	if err != nil {
		return types.Source{}, err
	}
	var data struct {
		ComputePlatform struct {
			Source types.Source `json:"source"`
		} `json:"computePlatform"`
	}
	vars := map[string]any{"sourceId": id}
	if err := p.tx.remoteFetch(ctx, "GetSource", p.id, q, vars, &data); err != nil {
		return types.Source{}, err
	}
	return data.ComputePlatform.Source, nil
}

// GetNamespaces returns the lite list of cluster namespaces.
func (p *ProxyClient) GetNamespaces(ctx context.Context) ([]types.K8sActualNamespace, error) {
	q, err := p.pick(&operations.GET_NAMESPACES)
	if err != nil {
		return nil, err
	}
	var data struct {
		ComputePlatform struct {
			K8sActualNamespaces []types.K8sActualNamespace `json:"k8sActualNamespaces"`
		} `json:"computePlatform"`
	}
	if err := p.tx.remoteFetch(ctx, "GetNamespaces", p.id, q, nil, &data); err != nil {
		return nil, err
	}
	return data.ComputePlatform.K8sActualNamespaces, nil
}

// GetNamespace returns the detailed view of a single namespace.
func (p *ProxyClient) GetNamespace(ctx context.Context, name string) (types.K8sActualNamespaceDetail, error) {
	q, err := p.pick(&operations.GET_NAMESPACE)
	if err != nil {
		return types.K8sActualNamespaceDetail{}, err
	}
	var data struct {
		ComputePlatform struct {
			K8sActualNamespace types.K8sActualNamespaceDetail `json:"k8sActualNamespace"`
		} `json:"computePlatform"`
	}
	vars := map[string]any{"namespaceName": name}
	if err := p.tx.remoteFetch(ctx, "GetNamespace", p.id, q, vars, &data); err != nil {
		return types.K8sActualNamespaceDetail{}, err
	}
	return data.ComputePlatform.K8sActualNamespace, nil
}

// PersistSources writes a batch of source-selection updates to the proxy.
// Returns the boolean success flag from the mutation.
func (p *ProxyClient) PersistSources(ctx context.Context, sources []types.SourceInput) (bool, error) {
	q, err := p.pick(&operations.PERSIST_SOURCES)
	if err != nil {
		return false, err
	}
	var data struct {
		PersistK8sSources bool `json:"persistK8sSources"`
	}
	vars := map[string]any{"sources": sources}
	if err := p.tx.remoteFetch(ctx, "PersistSources", p.id, q, vars, &data); err != nil {
		return false, err
	}
	return data.PersistK8sSources, nil
}

// PersistNamespaceSources writes a batch of namespace-selection updates.
func (p *ProxyClient) PersistNamespaceSources(ctx context.Context, namespaces []types.NamespaceInput) (bool, error) {
	q, err := p.pick(&operations.PERSIST_NAMESPACES)
	if err != nil {
		return false, err
	}
	var data struct {
		PersistK8sNamespaces bool `json:"persistK8sNamespaces"`
	}
	vars := map[string]any{"namespaces": namespaces}
	if err := p.tx.remoteFetch(ctx, "PersistNamespaceSources", p.id, q, vars, &data); err != nil {
		return false, err
	}
	return data.PersistK8sNamespaces, nil
}
