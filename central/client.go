// Package central is the runtime entry-point for the Odigos Central Go
// client. It exposes:
//
//   - CentralClient: a long-lived client that authenticates against Central,
//     performs a version handshake, and offers central-scoped operations
//     (GetComputePlatforms, GetSystemConfig).
//
//   - ProxyClient: a per-cluster handle, obtained via CentralClient.Proxy(id),
//     that carries the cluster's odigosVersion and platform type and offers
//     cluster-scoped operations (GetSources, GetNamespaces, etc.).
//
// All cluster-scoped operations route through Central's `remoteFetch` proxy.
// The right GraphQL document for a given proxy version is selected by
// operations.Operation.Pick — directly mirroring the central-ui behaviour.
package central

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/odigos-io/odigos-central-client/logger"
	"github.com/odigos-io/odigos-central-client/operations"
	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/types"
	"github.com/odigos-io/odigos-central-client/version"
)

// DefaultHTTPTimeout bounds requests made with the client NewClient creates
// when the caller does not supply their own *http.Client. It guards against a
// hung Central server holding a request open indefinitely; callers that need a
// different bound should pass a configured HTTPClient.
const DefaultHTTPTimeout = 30 * time.Second

// MinCentralVersion is the lowest Central server version this client knows
// how to talk to. It mirrors the K8s minimum used by the operations
// generator: anything older lacks the schema fields the v1.20+ variants
// expect to find.
var MinCentralVersion = version.MustParse("v1.20")

// MinProxyVersion is the lowest cluster proxy version we route remoteFetch
// calls to. Older proxies are rejected per-call via UnsupportedProxyVersionError
// so that Terraform runs spanning multiple clusters can still succeed for
// the supported ones.
var MinProxyVersion = version.MustParse("v1.20")

// CentralClient is the top-level Central API handle. Construct one per
// Terraform run (or per long-lived process) via NewClient.
type CentralClient struct {
	tx      *transport
	version version.Version

	platformsMu   sync.Mutex
	platforms     []types.ComputePlatform
	platformsRead bool
}

// ClientConfig captures everything NewClient needs.
type ClientConfig struct {
	// Hostname of the Central server (e.g. "central.example.com:8081").
	Hostname string
	// Username and Password identify a Central user and are sent through
	// the SignIn mutation. The returned access token is then attached to
	// every subsequent request.
	Username string
	Password string
	// HTTPClient is optional; if nil, a client bounded by DefaultHTTPTimeout
	// is used.
	HTTPClient *http.Client
	// Insecure switches the protocol to plain HTTP. False (HTTPS) is the
	// default and recommended for production.
	Insecure bool
	// InsecureSkipVerify keeps HTTPS but disables certificate and hostname
	// verification. The connection stays encrypted yet becomes open to
	// man-in-the-middle attacks, so this is only appropriate for a Central
	// server presenting a self-signed or otherwise untrusted certificate
	// (e.g. a dev or test cluster). It has no effect when Insecure is set,
	// since plain HTTP presents no certificate to verify.
	InsecureSkipVerify bool
}

// NewClient authenticates against Central, performs the version handshake,
// and returns a ready-to-use CentralClient.
//
// Returns:
//   - *UnsupportedCentralVersionError when the server is older than
//     MinCentralVersion (use errors.As to detect).
//   - *GraphQLError when the SignIn or systemConfig query fails.
//   - any transport-level error from the underlying http.Client.
func NewClient(ctx context.Context, cfg ClientConfig, opts ...logger.LoggingOption) (*CentralClient, error) {
	hc, err := httpClientFor(cfg)
	if err != nil {
		return nil, err
	}

	scheme := "https"
	if cfg.Insecure {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s/graphql", scheme, cfg.Hostname)

	logCfg := logger.SetLogConfig(opts)
	tx := newTransport(url, hc, logCfg.Logger)

	c := &CentralClient{tx: tx}

	if err := c.authenticate(ctx, cfg.Username, cfg.Password); err != nil {
		return nil, err
	}
	if err := c.handshake(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// httpClientFor resolves the *http.Client NewClient will use, applying
// cfg.InsecureSkipVerify on top of the caller's client if one was supplied.
// A caller-supplied client is never mutated: both it and its transport are
// copied before the TLS setting is applied, so the same client can be reused
// elsewhere with verification intact.
func httpClientFor(cfg ClientConfig) (*http.Client, error) {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	// Over plain HTTP there is no certificate to verify, so there is nothing
	// to apply and no reason to reject an unusual transport.
	if !cfg.InsecureSkipVerify || cfg.Insecure {
		return hc, nil
	}

	base := hc.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	ht, ok := base.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("InsecureSkipVerify requires HTTPClient.Transport to be *http.Transport (got %T); set TLSClientConfig.InsecureSkipVerify on your own transport instead", base)
	}

	// Clone deep-copies TLSClientConfig, so this cannot disable verification
	// for anything else sharing the original transport.
	cloned := ht.Clone()
	if cloned.TLSClientConfig == nil {
		cloned.TLSClientConfig = &tls.Config{}
	}
	cloned.TLSClientConfig.InsecureSkipVerify = true

	out := *hc
	out.Transport = cloned
	return &out, nil
}

// Version returns the Central server version detected during the handshake.
func (c *CentralClient) Version() version.Version { return c.version }

// signInData mirrors the SignIn mutation's payload shape.
type signInData struct {
	SignIn *types.SignInResult `json:"signIn"`
}

func (c *CentralClient) authenticate(ctx context.Context, username, password string) error {
	body := graphQLRequest{
		Query: operations.SIGN_IN,
		Variables: map[string]any{
			"email":    username,
			"password": password,
		},
	}
	var data signInData
	if err := c.tx.decodeData(ctx, "SignIn", body, &data); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	if data.SignIn == nil || data.SignIn.AccessToken == "" {
		return fmt.Errorf("authentication failed: server did not return an access token (check credentials)")
	}
	c.tx.token = data.SignIn.AccessToken
	return nil
}

func (c *CentralClient) handshake(ctx context.Context) error {
	body := graphQLRequest{
		Query:     operations.GET_SYSTEM_CONFIG,
		Variables: map[string]any{},
	}
	var data struct {
		SystemConfig types.SystemConfig `json:"systemConfig"`
	}
	if err := c.tx.decodeData(ctx, "GetSystemConfig", body, &data); err != nil {
		return fmt.Errorf("version handshake failed: %w", err)
	}
	if data.SystemConfig.Version == "" {
		return fmt.Errorf("version handshake failed: server reported empty systemConfig.version")
	}
	v, err := version.Parse(data.SystemConfig.Version)
	if err != nil {
		return fmt.Errorf("version handshake failed: %w", err)
	}
	if v.LT(MinCentralVersion) {
		return &UnsupportedCentralVersionError{Got: v, Min: MinCentralVersion}
	}
	c.version = v
	return nil
}

// GetComputePlatforms returns every cluster connected to Central. Successful
// results are cached; a failed read is not, so a transient error does not
// become sticky and a later call will retry. Use ResetPlatforms to force a
// fresh read after a successful one.
func (c *CentralClient) GetComputePlatforms(ctx context.Context) ([]types.ComputePlatform, error) {
	c.platformsMu.Lock()
	defer c.platformsMu.Unlock()
	return c.computePlatformsLocked(ctx, false)
}

// computePlatformsLocked returns the compute platforms, using the cache unless
// forceRefresh is set. Only successful reads populate the cache. Callers must
// hold platformsMu. The returned slice is a copy so callers cannot mutate the
// cached state.
func (c *CentralClient) computePlatformsLocked(ctx context.Context, forceRefresh bool) ([]types.ComputePlatform, error) {
	if c.platformsRead && !forceRefresh {
		return c.platformsCopyLocked(), nil
	}
	body := graphQLRequest{
		Query:     operations.GET_COMPUTE_PLATFORMS,
		Variables: map[string]any{},
	}
	var data struct {
		ComputePlatforms []types.ComputePlatform `json:"computePlatforms"`
	}
	if err := c.tx.decodeData(ctx, "GetComputePlatforms", body, &data); err != nil {
		return nil, err
	}
	c.platforms = data.ComputePlatforms
	c.platformsRead = true
	return c.platformsCopyLocked(), nil
}

// platformsCopyLocked returns a defensive copy of the cached platform slice.
// Callers must hold platformsMu.
func (c *CentralClient) platformsCopyLocked() []types.ComputePlatform {
	if c.platforms == nil {
		return nil
	}
	out := make([]types.ComputePlatform, len(c.platforms))
	copy(out, c.platforms)
	return out
}

// ResetPlatforms clears the cached compute-platform list. Useful when callers
// know that Central state has changed (e.g. a new cluster has joined) and
// want a fresh read. Rarely needed during a single Terraform run.
func (c *CentralClient) ResetPlatforms() {
	c.platformsMu.Lock()
	defer c.platformsMu.Unlock()
	c.platforms = nil
	c.platformsRead = false
}

// findPlatformBy locates a compute platform matching pred. If the platform is
// not found in a previously-cached list, the cache is refreshed once and the
// search retried, so a cluster that joined after the last read is still
// resolvable. desc describes the lookup key for the not-found error.
func (c *CentralClient) findPlatformBy(ctx context.Context, desc string, pred func(types.ComputePlatform) bool) (types.ComputePlatform, error) {
	c.platformsMu.Lock()
	defer c.platformsMu.Unlock()

	fromCache := c.platformsRead
	platforms, err := c.computePlatformsLocked(ctx, false)
	if err != nil {
		return types.ComputePlatform{}, err
	}
	if p, ok := findMatch(platforms, pred); ok {
		return p, nil
	}

	// A cold read already reflects the latest state, so only a cache hit
	// warrants a one-shot refresh before declaring the platform missing.
	if fromCache {
		platforms, err = c.computePlatformsLocked(ctx, true)
		if err != nil {
			return types.ComputePlatform{}, err
		}
		if p, ok := findMatch(platforms, pred); ok {
			return p, nil
		}
	}
	return types.ComputePlatform{}, fmt.Errorf("compute platform with %s not found", desc)
}

func findMatch(platforms []types.ComputePlatform, pred func(types.ComputePlatform) bool) (types.ComputePlatform, bool) {
	for _, p := range platforms {
		if pred(p) {
			return p, true
		}
	}
	return types.ComputePlatform{}, false
}

// findPlatform locates a compute platform by ID.
func (c *CentralClient) findPlatform(ctx context.Context, id string) (types.ComputePlatform, error) {
	return c.findPlatformBy(ctx, fmt.Sprintf("id %q", id), func(p types.ComputePlatform) bool {
		return p.ID == id
	})
}

// FindPlatformByName resolves a cluster name to a platform record. Useful
// for callers (like the Terraform provider) that key resources on
// cluster_name rather than the opaque proxy ID.
func (c *CentralClient) FindPlatformByName(ctx context.Context, name string) (types.ComputePlatform, error) {
	return c.findPlatformBy(ctx, fmt.Sprintf("name %q", name), func(p types.ComputePlatform) bool {
		return p.Name == name
	})
}

// Proxy returns a ProxyClient bound to the given compute-platform ID.
//
// Errors:
//   - cluster-not-found error if the ID isn't known to Central.
//   - *UnsupportedProxyPlatformError if the cluster's platform type is
//     unrecognised.
//   - *UnsupportedProxyVersionError if the cluster's odigosVersion is below
//     MinProxyVersion (callers should treat this as a per-cluster soft fail).
func (c *CentralClient) Proxy(ctx context.Context, proxyID string) (*ProxyClient, error) {
	cp, err := c.findPlatform(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	return c.proxyForPlatform(cp)
}

// ProxyByName is a convenience wrapper around FindPlatformByName + Proxy.
func (c *CentralClient) ProxyByName(ctx context.Context, name string) (*ProxyClient, error) {
	cp, err := c.FindPlatformByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return c.proxyForPlatform(cp)
}

func (c *CentralClient) proxyForPlatform(cp types.ComputePlatform) (*ProxyClient, error) {
	pt, err := platform.Parse(cp.Type)
	if err != nil {
		return nil, &UnsupportedProxyPlatformError{ProxyID: cp.ID, Platform: cp.Type}
	}
	v, err := version.Parse(cp.OdigosVersion)
	if err != nil {
		return nil, fmt.Errorf("proxy %q: %w", cp.ID, err)
	}
	if v.LT(MinProxyVersion) {
		return nil, &UnsupportedProxyVersionError{ProxyID: cp.ID, Platform: pt, Got: v, Min: MinProxyVersion}
	}
	return &ProxyClient{
		tx:           c.tx,
		id:           cp.ID,
		name:         cp.Name,
		version:      v,
		platformType: pt,
	}, nil
}

// IsUnsupported reports whether err is one of the structured "unsupported"
// errors emitted by this package or operations.Pick. Callers can use this to
// decide whether to surface a soft warning vs a hard error.
func IsUnsupported(err error) bool {
	var (
		eC *UnsupportedCentralVersionError
		eP *UnsupportedProxyPlatformError
		eV *UnsupportedProxyVersionError
		eO *operations.UnsupportedPlatformError
		eR *operations.UnsupportedVersionError
	)
	return errors.As(err, &eC) || errors.As(err, &eP) || errors.As(err, &eV) || errors.As(err, &eO) || errors.As(err, &eR)
}
