package central

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/odigos-io/odigos-central-client/types"
)

// fakeServer is a tiny GraphQL fixture: each call to Handle inspects the
// incoming request body's `query` field and returns a canned JSON response.
type fakeServer struct {
	t        *testing.T
	server   *httptest.Server
	handlers map[string]func(vars map[string]any) (any, []map[string]string)
}

func newFakeServer(t *testing.T) *fakeServer {
	fs := &fakeServer{
		t:        t,
		handlers: map[string]func(map[string]any) (any, []map[string]string){},
	}
	fs.server = httptest.NewServer(http.HandlerFunc(fs.serve))
	t.Cleanup(fs.server.Close)
	return fs
}

func (fs *fakeServer) URL() string {
	return strings.TrimPrefix(fs.server.URL, "http://")
}

// onOperation registers a fake response keyed by the GraphQL operation name.
// The handler returns the GraphQL `data` payload (will be marshalled) and a
// list of error messages (each rendered as {"message": ...}).
func (fs *fakeServer) onOperation(opName string, h func(vars map[string]any) (any, []map[string]string)) {
	fs.handlers[opName] = h
}

func (fs *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fs.t.Fatalf("fakeServer read body: %v", err)
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		fs.t.Fatalf("fakeServer unmarshal: %v", err)
	}
	op := opNameFromDoc(req.Query)
	h, ok := fs.handlers[op]
	if !ok {
		fs.t.Fatalf("fakeServer: unhandled operation %q\nquery=%q", op, req.Query)
	}
	data, errs := h(req.Variables)
	resp := map[string]any{}
	if data != nil {
		resp["data"] = data
	}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func opNameFromDoc(q string) string {
	q = strings.TrimSpace(q)
	prefix := ""
	switch {
	case strings.HasPrefix(q, "query"):
		prefix = "query"
	case strings.HasPrefix(q, "mutation"):
		prefix = "mutation"
	case strings.HasPrefix(q, "subscription"):
		prefix = "subscription"
	}
	if prefix == "" {
		return ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(q, prefix))
	end := strings.IndexAny(rest, " ({")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// remoteResponse builds the nested envelope returned by Central's remoteFetch:
// the outer GraphQL response carries `remoteFetch` as a JSON-encoded string
// whose body is `{"type":"graphql_response","data":{...},"request_id":"..."}`.
func remoteResponse(inner any) string {
	b, _ := json.Marshal(map[string]any{
		"type":       "graphql_response",
		"data":       inner,
		"request_id": "test-req-id",
	})
	return string(b)
}

// configureFakeAuthAndHandshake registers SignIn and GetSystemConfig handlers
// returning a v1.20 server.
func configureFakeAuthAndHandshake(fs *fakeServer, centralVersion string) {
	fs.onOperation("SignIn", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"signIn": map[string]any{
				"idToken":     "id-token",
				"accessToken": "access-token",
				"status":      "OK",
				"user":        map[string]any{"id": "1", "email": "u@example.com"},
			},
		}, nil
	})
	fs.onOperation("GetSystemConfig", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"systemConfig": map[string]any{
				"isInitialized": true,
				"version":       centralVersion,
			},
		}, nil
	})
}

func newClientForTest(t *testing.T, fs *fakeServer) *CentralClient {
	t.Helper()
	c, err := NewClient(context.Background(), ClientConfig{
		Hostname: fs.URL(),
		Username: "u@example.com",
		Password: "secret",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestNewClient_HappyPath(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22.0")

	c := newClientForTest(t, fs)
	if c.Version().String() != "v1.22" {
		t.Errorf("client version = %s, want v1.22", c.Version())
	}
}

func TestNewClient_RejectsOldCentral(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.18.0")

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: fs.URL(),
		Username: "u",
		Password: "p",
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
	var ucv *UnsupportedCentralVersionError
	if !errors.As(err, &ucv) {
		t.Fatalf("expected *UnsupportedCentralVersionError, got %T: %v", err, err)
	}
	if ucv.Got.String() != "v1.18" || ucv.Min.String() != "v1.20" {
		t.Errorf("unexpected error fields: %+v", ucv)
	}
	if !IsUnsupported(err) {
		t.Errorf("IsUnsupported should be true")
	}
}

func TestNewClient_BadCredentials(t *testing.T) {
	fs := newFakeServer(t)
	fs.onOperation("SignIn", func(vars map[string]any) (any, []map[string]string) {
		return nil, []map[string]string{{"message": "invalid credentials"}}
	})
	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: fs.URL(),
		Username: "u",
		Password: "wrong",
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gerr *GraphQLError
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *GraphQLError, got %T: %v", err, err)
	}
	if gerr.Op != "SignIn" {
		t.Errorf("op = %q", gerr.Op)
	}
}

func TestGetComputePlatforms_CachesResult(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	calls := 0
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		calls++
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
				{"id": "p2", "name": "beta", "odigosVersion": "v1.18.0", "type": "k8s"},
			},
		}, nil
	})
	c := newClientForTest(t, fs)

	platforms, err := c.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("GetComputePlatforms: %v", err)
	}
	if len(platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(platforms))
	}
	// Second call should not re-hit the server.
	_, _ = c.GetComputePlatforms(context.Background())
	if calls != 1 {
		t.Errorf("expected single GetComputePlatforms call, got %d", calls)
	}
}

func TestGetComputePlatforms_ErrorNotCached(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	calls := 0
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		calls++
		if calls == 1 {
			return nil, []map[string]string{{"message": "temporary failure"}}
		}
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	c := newClientForTest(t, fs)

	if _, err := c.GetComputePlatforms(context.Background()); err == nil {
		t.Fatal("expected first call to error")
	}
	// A transient failure must not be cached: a retry should re-hit the server
	// and succeed.
	platforms, err := c.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(platforms) != 1 {
		t.Fatalf("expected 1 platform after retry, got %d", len(platforms))
	}
	if calls != 2 {
		t.Errorf("expected 2 server calls, got %d", calls)
	}
}

func TestGetComputePlatforms_ReturnsDefensiveCopy(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	c := newClientForTest(t, fs)

	first, err := c.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("GetComputePlatforms: %v", err)
	}
	first[0].Name = "mutated"
	second, err := c.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("GetComputePlatforms: %v", err)
	}
	if second[0].Name != "alpha" {
		t.Errorf("cache was mutated via returned slice: got %q", second[0].Name)
	}
}

func TestProxy_RefreshesCacheOnMiss(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	includeSecond := false
	calls := 0
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		calls++
		list := []map[string]any{
			{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
		}
		if includeSecond {
			list = append(list, map[string]any{"id": "p2", "name": "beta", "odigosVersion": "v1.22.0", "type": "k8s"})
		}
		return map[string]any{"computePlatforms": list}, nil
	})
	c := newClientForTest(t, fs)

	// Prime the cache with only p1.
	if _, err := c.GetComputePlatforms(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}
	// p2 joins Central after the cache was populated.
	includeSecond = true
	p, err := c.Proxy(context.Background(), "p2")
	if err != nil {
		t.Fatalf("Proxy(p2) after refresh: %v", err)
	}
	if p.ID() != "p2" {
		t.Errorf("got proxy %q, want p2", p.ID())
	}
	if calls != 2 {
		t.Errorf("expected a one-shot refresh (2 calls), got %d", calls)
	}
}

func TestProxy_AcceptsSupportedAndRejectsOld(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "good", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
				{"id": "old", "name": "beta", "odigosVersion": "v1.18.0", "type": "k8s"},
				{"id": "weird", "name": "gamma", "odigosVersion": "v1.22.0", "type": "vmx"},
			},
		}, nil
	})
	c := newClientForTest(t, fs)

	good, err := c.Proxy(context.Background(), "good")
	if err != nil {
		t.Fatalf("Proxy(good): %v", err)
	}
	if good.Version().String() != "v1.22" {
		t.Errorf("good version = %s", good.Version())
	}

	_, err = c.Proxy(context.Background(), "old")
	var upe *UnsupportedProxyVersionError
	if !errors.As(err, &upe) {
		t.Fatalf("Proxy(old): expected *UnsupportedProxyVersionError, got %T: %v", err, err)
	}

	_, err = c.Proxy(context.Background(), "weird")
	var ppe *UnsupportedProxyPlatformError
	if !errors.As(err, &ppe) {
		t.Fatalf("Proxy(weird): expected *UnsupportedProxyPlatformError, got %T: %v", err, err)
	}
}

func TestProxy_GetSourcesViaRemoteFetch(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	fs.onOperation("RemoteFetch", func(vars map[string]any) (any, []map[string]string) {
		// Inner GraphQL response with two sources.
		inner := map[string]any{
			"computePlatform": map[string]any{
				"sources": []map[string]any{
					{
						"namespace":       "default",
						"name":            "demo",
						"kind":            "Deployment",
						"selected":        true,
						"otelServiceName": "demo",
						"dataStreamNames": []string{"default"},
					},
					{
						"namespace":       "team",
						"name":            "api",
						"kind":            "Deployment",
						"selected":        false,
						"otelServiceName": "api",
						"dataStreamNames": []string{},
					},
				},
			},
		}
		return map[string]any{"remoteFetch": remoteResponse(inner)}, nil
	})

	c := newClientForTest(t, fs)
	p, err := c.Proxy(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	sources, err := p.GetSources(context.Background())
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %d sources", len(sources))
	}
	if sources[0].OtelServiceName != "demo" {
		t.Errorf("unexpected first source: %+v", sources[0])
	}
	if sources[0].ManifestYAML != nil {
		t.Errorf("manifestYAML should be nil for list payload, got %v", *sources[0].ManifestYAML)
	}
}

func TestProxy_GetSourceDetailIncludesYAMLs(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	fs.onOperation("GetSource", func(vars map[string]any) (any, []map[string]string) {
		return nil, []map[string]string{{"message": "should be reached via remoteFetch"}}
	})
	fs.onOperation("RemoteFetch", func(vars map[string]any) (any, []map[string]string) {
		manifest := "kind: Deployment\nname: demo\n"
		instr := "instrumentation: yaml\n"
		inner := map[string]any{
			"computePlatform": map[string]any{
				"source": map[string]any{
					"namespace":                 "default",
					"name":                      "demo",
					"kind":                      "Deployment",
					"selected":                  true,
					"otelServiceName":           "demo",
					"dataStreamNames":           []string{"default"},
					"manifestYAML":              manifest,
					"instrumentationConfigYAML": instr,
				},
			},
		}
		return map[string]any{"remoteFetch": remoteResponse(inner)}, nil
	})

	c := newClientForTest(t, fs)
	p, err := c.Proxy(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}
	src, err := p.GetSource(context.Background(), types.SourceID{Kind: "Deployment", Name: "demo", Namespace: "default"})
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if src.ManifestYAML == nil || *src.ManifestYAML == "" {
		t.Errorf("expected manifestYAML to be populated")
	}
	if src.InstrumentationConfigYAML == nil || *src.InstrumentationConfigYAML == "" {
		t.Errorf("expected instrumentationConfigYAML to be populated")
	}
}

func TestPersistSources_HappyPath(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	fs.onOperation("RemoteFetch", func(vars map[string]any) (any, []map[string]string) {
		inner := map[string]any{"persistK8sSources": true}
		return map[string]any{"remoteFetch": remoteResponse(inner)}, nil
	})

	c := newClientForTest(t, fs)
	p, err := c.Proxy(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Proxy: %v", err)
	}

	ok, err := p.PersistSources(context.Background(), nil)
	if err != nil {
		t.Fatalf("PersistSources: %v", err)
	}
	if !ok {
		t.Errorf("expected ok=true")
	}
}

func TestRemoteFetch_PropagatesInnerErrors(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	fs.onOperation("GetComputePlatforms", func(vars map[string]any) (any, []map[string]string) {
		return map[string]any{
			"computePlatforms": []map[string]any{
				{"id": "p1", "name": "alpha", "odigosVersion": "v1.22.0", "type": "k8s"},
			},
		}, nil
	})
	fs.onOperation("RemoteFetch", func(vars map[string]any) (any, []map[string]string) {
		inner, _ := json.Marshal(map[string]any{
			"type":       "graphql_response",
			"data":       map[string]any{"errors": []map[string]any{{"message": "namespace not found"}}},
			"request_id": "rid",
		})
		return map[string]any{"remoteFetch": string(inner)}, nil
	})

	c := newClientForTest(t, fs)
	p, _ := c.Proxy(context.Background(), "p1")
	_, err := p.GetNamespace(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected GraphQL error, got nil")
	}
	var ge *GraphQLError
	if !errors.As(err, &ge) {
		t.Fatalf("expected *GraphQLError, got %T: %v", err, err)
	}
	if !strings.Contains(ge.Error(), "namespace not found") {
		t.Errorf("unexpected error: %v", ge)
	}
}

func TestToGraphQLError_CapturesStructuredFields(t *testing.T) {
	errs := []graphqlError{{
		Message:    "boom",
		Path:       []any{"computePlatform", "sources"},
		Locations:  []graphqlErrorLocation{{Line: 3, Column: 5}},
		Extensions: map[string]any{"code": "INTERNAL"},
	}}
	ge := toGraphQLError("GetSources", errs)
	if ge.Op != "GetSources" || len(ge.Errors) != 1 {
		t.Fatalf("unexpected error: %+v", ge)
	}
	d := ge.Errors[0]
	if d.Message != "boom" {
		t.Errorf("message = %q", d.Message)
	}
	if len(d.Path) != 2 {
		t.Errorf("path = %v", d.Path)
	}
	if len(d.Locations) != 1 || d.Locations[0].Line != 3 || d.Locations[0].Column != 5 {
		t.Errorf("locations = %v", d.Locations)
	}
	if d.Extensions["code"] != "INTERNAL" {
		t.Errorf("extensions = %v", d.Extensions)
	}
	if msgs := ge.Messages(); len(msgs) != 1 || msgs[0] != "boom" {
		t.Errorf("Messages() = %v", msgs)
	}
}

func TestSanitizeBodyRedactsPassword(t *testing.T) {
	body := graphQLRequest{
		Query: "mutation X { x }",
		Variables: map[string]any{
			"email":    "u@example.com",
			"password": "super-secret",
		},
	}
	got := sanitizeBody(body)
	if strings.Contains(got, "super-secret") {
		t.Errorf("password leaked in sanitized body: %s", got)
	}
	if !strings.Contains(got, "u@example.com") {
		t.Errorf("email should not be redacted: %s", got)
	}
}

func TestSanitizeBodyRedactsCode(t *testing.T) {
	body := graphQLRequest{
		Query:     "mutation SignIn { x }",
		Variables: map[string]any{"code": "123456"},
	}
	if got := sanitizeBody(body); strings.Contains(got, "123456") {
		t.Errorf("2FA code leaked in sanitized body: %s", got)
	}
}

func TestSanitizeResponseRedactsTokens(t *testing.T) {
	raw := []byte(`{"data":{"signIn":{"idToken":"id-abc","accessToken":"acc-xyz","status":"OK"}}}`)
	got := sanitizeResponse(raw)
	for _, secret := range []string{"id-abc", "acc-xyz"} {
		if strings.Contains(got, secret) {
			t.Errorf("token %q leaked in sanitized response: %s", secret, got)
		}
	}
	if !strings.Contains(got, `"status":"OK"`) {
		t.Errorf("non-sensitive fields should be preserved: %s", got)
	}
}

// Sanity: ensure the fake server constructed an http server reachable.
func TestFakeServerReachable(t *testing.T) {
	fs := newFakeServer(t)
	configureFakeAuthAndHandshake(fs, "v1.22")
	url := fmt.Sprintf("http://%s/graphql", fs.URL())
	resp, err := http.Post(url, "application/json", strings.NewReader(`{"query":"query GetSystemConfig { x }","variables":{}}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
