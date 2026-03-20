package odigos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/odigos-io/odigos-central-client/logger"
)

// noopLogger satisfies logger.CustomLogger but discards all output.
type noopLogger struct{}

func (noopLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (noopLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (noopLogger) Error(_ context.Context, _ string, _ ...any) {}

// newTestClient constructs an OdigosClient wired to a test server with a
// noop logger, suitable for unit-testing methods that call execute().
func newTestClient(url string, httpClient *http.Client, token string) *OdigosClient {
	return &OdigosClient{
		url:        url,
		httpClient: httpClient,
		token:      token,
		logger:     noopLogger{},
	}
}

// ---------------------------------------------------------------------------
// helpers for building test servers
// ---------------------------------------------------------------------------

// newTestServer returns an *httptest.Server whose handler replies with the
// given status code and body for every request. The caller should defer
// ts.Close().
func newTestServer(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, body)
	}))
}

// newTestServerFunc returns an *httptest.Server that delegates to the supplied
// handler function.
func newTestServerFunc(fn http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(fn)
}

// trimScheme strips "http://" or "https://" so we can use the result as
// ClientConfig.Hostname for tests running against httptest servers.
func trimScheme(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "https://")
	// Remove any trailing /graphql if present
	rawURL = strings.TrimSuffix(rawURL, "/graphql")
	return rawURL
}

// authSuccessBody returns a valid signIn JSON response body.
func authSuccessBody() string {
	return `{
		"data": {
			"signIn": {
				"accessToken": "test-token-123",
				"idToken": "id-token-456",
				"status": "ok",
				"user": {
					"id": "u1",
					"email": "user@example.com",
					"username": "testuser",
					"role": "admin",
					"needsPasswordChange": false,
					"teams": [],
					"computePlatforms": [],
					"__typename": "User"
				},
				"__typename": "AuthPayload"
			}
		}
	}`
}

// ---------------------------------------------------------------------------
// authQuery tests
// ---------------------------------------------------------------------------

func TestAuthQuery(t *testing.T) {
	cfg := ClientConfig{
		Username: "alice@example.com",
		Password: "s3cret",
	}
	query, vars := authQuery(cfg)

	if query != SignInMutation {
		t.Errorf("expected query to be SignInMutation, got %q", query)
	}
	if vars["email"] != cfg.Username {
		t.Errorf("expected email=%q, got %q", cfg.Username, vars["email"])
	}
	if vars["password"] != cfg.Password {
		t.Errorf("expected password=%q, got %q", cfg.Password, vars["password"])
	}
}

func TestAuthQuery_EmptyCredentials(t *testing.T) {
	cfg := ClientConfig{}
	query, vars := authQuery(cfg)

	if query != SignInMutation {
		t.Errorf("expected query to be SignInMutation, got %q", query)
	}
	if vars["email"] != "" {
		t.Errorf("expected empty email, got %q", vars["email"])
	}
	if vars["password"] != "" {
		t.Errorf("expected empty password, got %q", vars["password"])
	}
}

// ---------------------------------------------------------------------------
// NewClient tests
// ---------------------------------------------------------------------------

func TestNewClient_Success(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Username: "user@example.com",
		Password: "password",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.token != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got %q", client.token)
	}
}

func TestNewClient_DefaultHTTPClient(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.httpClient != http.DefaultClient {
		t.Error("expected default HTTP client to be used when none provided")
	}
}

func TestNewClient_CustomHTTPClient(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	custom := &http.Client{Timeout: 42 * time.Second}
	client, err := NewClient(context.Background(), ClientConfig{
		Hostname:   trimScheme(ts.URL),
		HTTPClient: custom,
		Insecure:   true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.httpClient != custom {
		t.Error("expected custom HTTP client to be used")
	}
}

func TestNewClient_SecureURL(t *testing.T) {
	// We can't easily test real HTTPS with httptest.NewServer, but we can
	// verify the URL is constructed correctly by inspecting the error (the
	// TLS handshake will fail against a plain-HTTP server).
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: false, // will produce https:// URL
	})
	// We expect an error because the server is plain HTTP
	if err == nil {
		t.Fatal("expected error when using HTTPS against plain HTTP server")
	}
}

func TestNewClient_InsecureURL(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	expectedURL := fmt.Sprintf("http://%s/graphql", trimScheme(ts.URL))
	if client.url != expectedURL {
		t.Errorf("expected url=%q, got %q", expectedURL, client.url)
	}
}

func TestNewClient_AuthFailure_GraphQLErrors(t *testing.T) {
	body := `{
		"data": {"signIn": null},
		"errors": [{"message": "invalid credentials"}, {"message": "account locked"}]
	}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("expected error to contain 'invalid credentials', got: %v", err)
	}
	if !strings.Contains(err.Error(), "account locked") {
		t.Errorf("expected error to contain 'account locked', got: %v", err)
	}
}

func TestNewClient_AuthFailure_NilSignIn(t *testing.T) {
	body := `{"data": {"signIn": null}}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error for nil signIn")
	}
	if !strings.Contains(err.Error(), "no access token") {
		t.Errorf("expected error about missing access token, got: %v", err)
	}
}

func TestNewClient_AuthFailure_EmptyAccessToken(t *testing.T) {
	body := `{
		"data": {
			"signIn": {
				"accessToken": "",
				"idToken": "id",
				"status": "ok",
				"user": {"id": "u1", "email": "", "username": "", "role": "", "needsPasswordChange": false, "teams": [], "computePlatforms": [], "__typename": "User"},
				"__typename": "AuthPayload"
			}
		}
	}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
	if !strings.Contains(err.Error(), "no access token") {
		t.Errorf("expected error about missing access token, got: %v", err)
	}
}

func TestNewClient_ServerError(t *testing.T) {
	ts := newTestServer(http.StatusInternalServerError, `internal server error`)
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status code 500, got: %v", err)
	}
}

func TestNewClient_CancelledContext(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := NewClient(ctx, ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestNewClient_InvalidJSON(t *testing.T) {
	ts := newTestServer(http.StatusOK, `not json`)
	defer ts.Close()

	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to parse auth response") {
		t.Errorf("expected wrapped auth parse error, got: %v", err)
	}
}

func TestNewClient_UnreachableHost(t *testing.T) {
	_, err := NewClient(context.Background(), ClientConfig{
		Hostname: "192.0.2.1:1", // TEST-NET-1, should be unreachable
		Insecure: true,
		HTTPClient: &http.Client{
			Timeout: 100 * time.Millisecond,
		},
	})
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}

// ---------------------------------------------------------------------------
// execute tests
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	ts := newTestServer(http.StatusOK, `{"data":"ok"}`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "test-token")
	body, err := client.execute(context.Background(), graphQLRequest{
		Query: "query { hello }",
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if string(body) != `{"data":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestExecute_SetsAuthorizationHeader(t *testing.T) {
	var receivedAuth string
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "my-token")
	_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if receivedAuth != "Bearer my-token" {
		t.Errorf("expected Authorization='Bearer my-token', got %q", receivedAuth)
	}
}

func TestExecute_NoAuthHeaderWhenTokenEmpty(t *testing.T) {
	var receivedAuth string
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if receivedAuth != "" {
		t.Errorf("expected no Authorization header, got %q", receivedAuth)
	}
}

func TestExecute_ContentType(t *testing.T) {
	var receivedCT string
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if receivedCT != "application/json" {
		t.Errorf("expected Content-Type='application/json', got %q", receivedCT)
	}
}

func TestExecute_SendsCorrectBody(t *testing.T) {
	var receivedBody []byte
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")

	reqBody := graphQLRequest{
		Query:     "query { test }",
		Variables: map[string]interface{}{"key": "val"},
	}
	_, err := client.execute(context.Background(), reqBody)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	var decoded graphQLRequest
	if err := json.Unmarshal(receivedBody, &decoded); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}
	if decoded.Query != reqBody.Query {
		t.Errorf("expected query=%q, got %q", reqBody.Query, decoded.Query)
	}
}

func TestExecute_NonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"403 Forbidden", http.StatusForbidden},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(tt.statusCode, `error body`)
			defer ts.Close()

			client := newTestClient(ts.URL, ts.Client(), "")
			_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
			if err == nil {
				t.Fatalf("expected error for status %d", tt.statusCode)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", tt.statusCode)) {
				t.Errorf("expected error to contain status code %d, got: %v", tt.statusCode, err)
			}
		})
	}
}

func TestExecute_CancelledContext(t *testing.T) {
	ts := newTestServer(http.StatusOK, `{}`)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newTestClient(ts.URL, ts.Client(), "")
	_, err := client.execute(ctx, graphQLRequest{Query: "q"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestExecute_TimeoutContext(t *testing.T) {
	// Server that takes a long time
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	})
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := newTestClient(ts.URL, ts.Client(), "")
	_, err := client.execute(ctx, graphQLRequest{Query: "q"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExecute_MarshalError(t *testing.T) {
	ts := newTestServer(http.StatusOK, `{}`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	// A channel value cannot be marshaled to JSON, triggering the marshal error path
	_, err := client.execute(context.Background(), graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{"bad": make(chan int)},
	})
	if err == nil {
		t.Fatal("expected error for unmarshalable request body")
	}
	if !strings.Contains(err.Error(), "failed to marshal request") {
		t.Errorf("expected 'failed to marshal request' error, got: %v", err)
	}
}

func TestExecute_InvalidURL(t *testing.T) {
	client := newTestClient("://invalid-url", http.DefaultClient, "")
	_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// errorReadCloser returns an error after reading partialData.
type errorReadCloser struct {
	data []byte
	pos  int
}

func (e *errorReadCloser) Read(p []byte) (int, error) {
	if e.pos >= len(e.data) {
		return 0, fmt.Errorf("simulated read error")
	}
	n := copy(p, e.data[e.pos:])
	e.pos += n
	return n, fmt.Errorf("simulated read error")
}

func (e *errorReadCloser) Close() error { return nil }

// errorBodyTransport is an http.RoundTripper that returns a response with an
// erroring body to exercise the io.ReadAll error path.
type errorBodyTransport struct{}

func (t *errorBodyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       &errorReadCloser{data: []byte("partial")},
		Header:     make(http.Header),
	}, nil
}

func TestExecute_ReadBodyError(t *testing.T) {
	client := newTestClient("http://localhost/graphql", &http.Client{Transport: &errorBodyTransport{}}, "")
	_, err := client.execute(context.Background(), graphQLRequest{Query: "q"})
	if err == nil {
		t.Fatal("expected error when body read fails")
	}
	if !strings.Contains(err.Error(), "failed to read response") {
		t.Errorf("expected 'failed to read response' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// authenticate tests
// ---------------------------------------------------------------------------

func TestAuthenticate_Success(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{
		Query:     SignInMutation,
		Variables: map[string]interface{}{"email": "a@b.com", "password": "pwd"},
	})
	if err != nil {
		t.Fatalf("authenticate returned error: %v", err)
	}
	if client.token != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got %q", client.token)
	}
}

func TestAuthenticate_GraphQLErrors(t *testing.T) {
	body := `{
		"data": {"signIn": null},
		"errors": [{"message": "bad creds"}]
	}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bad creds") {
		t.Errorf("expected error to contain 'bad creds', got: %v", err)
	}
}

func TestAuthenticate_MultipleGraphQLErrors(t *testing.T) {
	body := `{
		"data": {"signIn": null},
		"errors": [{"message": "error1"}, {"message": "error2"}, {"message": "error3"}]
	}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error")
	}
	// All error messages should be joined with "; "
	if !strings.Contains(err.Error(), "error1; error2; error3") {
		t.Errorf("expected all error messages joined, got: %v", err)
	}
}

func TestAuthenticate_NilSignIn(t *testing.T) {
	body := `{"data": {"signIn": null}}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error for nil signIn")
	}
	if !strings.Contains(err.Error(), "no access token") {
		t.Errorf("expected 'no access token' error, got: %v", err)
	}
}

func TestAuthenticate_EmptyAccessToken(t *testing.T) {
	body := `{
		"data": {
			"signIn": {
				"accessToken": "",
				"idToken": "id",
				"status": "ok",
				"user": {"id":"u","email":"","username":"","role":"","needsPasswordChange":false,"teams":[],"computePlatforms":[],"__typename":"User"},
				"__typename": "AuthPayload"
			}
		}
	}`
	ts := newTestServer(http.StatusOK, body)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
}

func TestAuthenticate_InvalidJSON(t *testing.T) {
	ts := newTestServer(http.StatusOK, `not-json`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse auth response") {
		t.Errorf("expected wrapped auth parse error, got: %v", err)
	}
}

func TestAuthenticate_ExecuteError(t *testing.T) {
	ts := newTestServer(http.StatusBadGateway, `bad gateway`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")
	err := client.authenticate(context.Background(), graphQLRequest{Query: SignInMutation})
	if err == nil {
		t.Fatal("expected error from execute")
	}
}

// ---------------------------------------------------------------------------
// GetComputePlatforms tests
// ---------------------------------------------------------------------------

func TestGetComputePlatforms_Success(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// auth request
			fmt.Fprint(w, authSuccessBody())
		} else {
			// GetComputePlatforms request
			fmt.Fprint(w, `{
				"data": {
					"computePlatforms": [
						{
							"id": "cp-1",
							"name": "platform-1",
							"odigosVersion": "1.0.0",
							"type": "K8S",
							"status": "CONNECTED",
							"connectedAt": 1700000000,
							"lastSeenAt": 1700000100,
							"users": [],
							"teams": [],
							"__typename": "ComputePlatform"
						},
						{
							"id": "cp-2",
							"name": "platform-2",
							"odigosVersion": "1.1.0",
							"type": "K8S",
							"status": "DISCONNECTED",
							"connectedAt": 1700000200,
							"lastSeenAt": 1700000300,
							"users": [],
							"teams": [],
							"__typename": "ComputePlatform"
						}
					]
				}
			}`)
		}
	})
	defer ts.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	platforms, err := client.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("GetComputePlatforms returned error: %v", err)
	}
	if len(platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(platforms))
	}
	if platforms[0].ID != "cp-1" {
		t.Errorf("expected first platform ID='cp-1', got %q", platforms[0].ID)
	}
	if platforms[0].Name != "platform-1" {
		t.Errorf("expected first platform Name='platform-1', got %q", platforms[0].Name)
	}
	if platforms[1].Status != "DISCONNECTED" {
		t.Errorf("expected second platform Status='DISCONNECTED', got %q", platforms[1].Status)
	}
}

func TestGetComputePlatforms_EmptyList(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, `{"data": {"computePlatforms": []}}`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	platforms, err := client.GetComputePlatforms(context.Background())
	if err != nil {
		t.Fatalf("GetComputePlatforms returned error: %v", err)
	}
	if len(platforms) != 0 {
		t.Errorf("expected empty list, got %d", len(platforms))
	}
}

func TestGetComputePlatforms_ServerError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `server error`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetComputePlatforms(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetComputePlatforms_InvalidJSON(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, `{invalid json`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetComputePlatforms(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetComputePlatforms_GraphQLErrors(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, `{"data":{"computePlatforms":null},"errors":[{"message":"unauthorized access"}]}`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetComputePlatforms(context.Background())
	if err == nil {
		t.Fatal("expected error for GraphQL errors")
	}
	if !strings.Contains(err.Error(), "unauthorized access") {
		t.Errorf("expected error to contain 'unauthorized access', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetNamespaces tests
// ---------------------------------------------------------------------------

func remoteFetchResponse(innerJSON string) string {
	// The remote fetch pattern: outer GraphQL wraps a JSON string
	escaped, _ := json.Marshal(innerJSON)
	return fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))
}

func TestGetNamespaces_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"k8sActualNamespaces": [
					{"name": "default", "selected": true, "dataStreamNames": []},
					{"name": "kube-system", "selected": false, "dataStreamNames": []}
				]
			}
		},
		"request_id": "req-1"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	namespaces, err := client.GetNamespaces(context.Background(), "proxy-1")
	if err != nil {
		t.Fatalf("GetNamespaces returned error: %v", err)
	}
	if len(namespaces) != 2 {
		t.Fatalf("expected 2 namespaces, got %d", len(namespaces))
	}
	if namespaces[0].Name != "default" {
		t.Errorf("expected first namespace='default', got %q", namespaces[0].Name)
	}
	if !namespaces[0].Selected {
		t.Error("expected first namespace to be selected")
	}
	if namespaces[1].Name != "kube-system" {
		t.Errorf("expected second namespace='kube-system', got %q", namespaces[1].Name)
	}
	if namespaces[1].Selected {
		t.Error("expected second namespace to not be selected")
	}
}

func TestGetNamespaces_EmptyList(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"computePlatform": {"k8sActualNamespaces": []}},
		"request_id": "req-1"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	namespaces, err := client.GetNamespaces(context.Background(), "proxy-1")
	if err != nil {
		t.Fatalf("GetNamespaces returned error: %v", err)
	}
	if len(namespaces) != 0 {
		t.Errorf("expected 0 namespaces, got %d", len(namespaces))
	}
}

func TestGetNamespaces_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `error`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetNamespaces(context.Background(), "proxy-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// GetSources tests
// ---------------------------------------------------------------------------

func TestGetSources_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"sources": [
					{
						"name": "my-app",
						"namespace": "default",
						"kind": "Deployment",
						"dataStreamNames": ["stream-1"],
						"selected": true,
						"otelServiceName": "my-app-svc",
						"containers": [
							{
								"containerName": "main",
								"language": "go",
								"runtimeVersion": "1.21",
								"overriden": false,
								"instrumented": true,
								"instrumentationMessage": "",
								"otelDistroName": "odigos"
							}
						],
						"conditions": [
							{
								"status": "True",
								"type": "Ready",
								"reason": "InstrumentationComplete",
								"message": "ok",
								"lastTransitionTime": "2024-01-01T00:00:00Z"
							}
						]
					}
				]
			}
		},
		"request_id": "req-2"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	sources, err := client.GetSources(context.Background(), "proxy-1")
	if err != nil {
		t.Fatalf("GetSources returned error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Name != "my-app" {
		t.Errorf("expected name='my-app', got %q", sources[0].Name)
	}
	if sources[0].Kind != "Deployment" {
		t.Errorf("expected kind='Deployment', got %q", sources[0].Kind)
	}
	if len(sources[0].Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(sources[0].Containers))
	}
	if sources[0].Containers[0].Language != "go" {
		t.Errorf("expected language='go', got %q", sources[0].Containers[0].Language)
	}
	if len(sources[0].Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(sources[0].Conditions))
	}
	if sources[0].Conditions[0].Status != "True" {
		t.Errorf("expected condition status='True', got %q", sources[0].Conditions[0].Status)
	}
}

func TestGetSources_EmptyList(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"computePlatform": {"sources": []}},
		"request_id": "req-2"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	sources, err := client.GetSources(context.Background(), "proxy-1")
	if err != nil {
		t.Fatalf("GetSources returned error: %v", err)
	}
	if len(sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(sources))
	}
}

func TestGetSources_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `unavailable`)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetSources(context.Background(), "proxy-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// GetSource tests
// ---------------------------------------------------------------------------

func TestGetSource_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"source": {
					"name": "my-svc",
					"namespace": "production",
					"kind": "StatefulSet",
					"dataStreamNames": ["stream-a", "stream-b"],
					"selected": false,
					"otelServiceName": "my-svc-otel",
					"containers": [],
					"conditions": []
				}
			}
		},
		"request_id": "req-3"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	source, err := client.GetSource(context.Background(), "proxy-1", SourceId{
		Kind:      "StatefulSet",
		Name:      "my-svc",
		Namespace: "production",
	})
	if err != nil {
		t.Fatalf("GetSource returned error: %v", err)
	}
	if source.Name != "my-svc" {
		t.Errorf("expected name='my-svc', got %q", source.Name)
	}
	if source.Namespace != "production" {
		t.Errorf("expected namespace='production', got %q", source.Namespace)
	}
	if source.Kind != "StatefulSet" {
		t.Errorf("expected kind='StatefulSet', got %q", source.Kind)
	}
	if len(source.DataStreamNames) != 2 {
		t.Errorf("expected 2 data stream names, got %d", len(source.DataStreamNames))
	}
}

func TestGetSource_PassesSourceIdInVariables(t *testing.T) {
	var receivedBody []byte
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			receivedBody, _ = io.ReadAll(r.Body)
			inner := `{
				"type": "data",
				"data": {"computePlatform": {"source": {"name":"s","namespace":"n","kind":"k","dataStreamNames":[],"selected":false,"otelServiceName":"","containers":[],"conditions":[]}}},
				"request_id": "r"
			}`
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetSource(context.Background(), "proxy-1", SourceId{
		Kind:      "Deployment",
		Name:      "web",
		Namespace: "staging",
	})
	if err != nil {
		t.Fatalf("GetSource returned error: %v", err)
	}

	// Verify the variables contain proxyID and the inner variables with sourceId
	var req graphQLRequest
	if err := json.Unmarshal(receivedBody, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}
	if req.Variables["proxyID"] != "proxy-1" {
		t.Errorf("expected proxyID='proxy-1', got %v", req.Variables["proxyID"])
	}
	vars, ok := req.Variables["variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected variables to be a map, got %T", req.Variables["variables"])
	}
	sourceId, ok := vars["sourceId"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sourceId to be a map, got %T", vars["sourceId"])
	}
	if sourceId["kind"] != "Deployment" {
		t.Errorf("expected sourceId.kind='Deployment', got %v", sourceId["kind"])
	}
}

func TestGetSource_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	source, err := client.GetSource(context.Background(), "proxy-1", SourceId{})
	if err == nil {
		t.Fatal("expected error")
	}
	// On error, should return zero-value Source
	if source.Name != "" || source.Kind != "" || source.Namespace != "" {
		t.Error("expected zero-value Source on error")
	}
}

// ---------------------------------------------------------------------------
// GetNamespace tests
// ---------------------------------------------------------------------------

func TestGetNamespace_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"k8sActualNamespace": {
					"name": "default",
					"selected": true,
					"dataStreamNames": ["logs", "metrics"],
					"sources": [
						{
							"namespace": "default",
							"kind": "Deployment",
							"name": "my-app",
							"dataStreamNames": ["logs"],
							"selected": true,
							"numberOfInstances": 3
						}
					]
				}
			}
		},
		"request_id": "req-ns"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	ns, err := client.GetNamespace(context.Background(), "proxy-1", "default")
	if err != nil {
		t.Fatalf("GetNamespace returned error: %v", err)
	}
	if ns.Name != "default" {
		t.Errorf("expected name='default', got %q", ns.Name)
	}
	if !ns.Selected {
		t.Error("expected selected=true")
	}
	if len(ns.DataStreamNames) != 2 {
		t.Errorf("expected 2 data stream names, got %d", len(ns.DataStreamNames))
	}
	if len(ns.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(ns.Sources))
	}
	if ns.Sources[0].Name != "my-app" || ns.Sources[0].Kind != "Deployment" || ns.Sources[0].Namespace != "default" {
		t.Errorf("expected source my-app Deployment default, got %q %q %q", ns.Sources[0].Name, ns.Sources[0].Kind, ns.Sources[0].Namespace)
	}
	if ns.Sources[0].NumberOfInstances != 3 {
		t.Errorf("expected numberOfInstances=3, got %d", ns.Sources[0].NumberOfInstances)
	}
}

func TestGetNamespace_PassesNamespaceNameInVariables(t *testing.T) {
	var receivedBody []byte
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			receivedBody, _ = io.ReadAll(r.Body)
			inner := `{
				"type": "data",
				"data": {"computePlatform": {"k8sActualNamespace": {"name":"kube-system","selected":false,"dataStreamNames":[],"sources":[]}}},
				"request_id": "r"
			}`
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	_, err := client.GetNamespace(context.Background(), "proxy-1", "kube-system")
	if err != nil {
		t.Fatalf("GetNamespace returned error: %v", err)
	}

	var req graphQLRequest
	if err := json.Unmarshal(receivedBody, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}
	if req.Variables["proxyID"] != "proxy-1" {
		t.Errorf("expected proxyID='proxy-1', got %v", req.Variables["proxyID"])
	}
	vars, ok := req.Variables["variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected variables to be a map, got %T", req.Variables["variables"])
	}
	if vars["namespaceName"] != "kube-system" {
		t.Errorf("expected namespaceName='kube-system', got %v", vars["namespaceName"])
	}
}

func TestGetNamespace_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	ns, err := client.GetNamespace(context.Background(), "proxy-1", "default")
	if err == nil {
		t.Fatal("expected error")
	}
	if ns.Name != "" || len(ns.Sources) != 0 {
		t.Error("expected zero-value K8sActualNamespaceDetail on error")
	}
}

// ---------------------------------------------------------------------------
// PersistSources tests
// ---------------------------------------------------------------------------

func TestPersistSources_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sSources": true, "errors": []},
		"request_id": "req-4"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistSources(context.Background(), "proxy-1", []SourceInput{
		{
			CurrentStreamName: "stream-1",
			Kind:              "Deployment",
			Name:              "my-app",
			Namespace:         "default",
			Selected:          true,
		},
	})
	if err != nil {
		t.Fatalf("PersistSources returned error: %v", err)
	}
	if !result {
		t.Error("expected result to be true")
	}
}

func TestPersistSources_False(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sSources": false, "errors": []},
		"request_id": "req-4"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistSources(context.Background(), "proxy-1", nil)
	if err != nil {
		t.Fatalf("PersistSources returned error: %v", err)
	}
	if result {
		t.Error("expected result to be false")
	}
}

func TestPersistSources_WithErrors(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"persistK8sSources": false,
			"errors": [{"message": "source not found"}, {"message": "permission denied"}]
		},
		"request_id": "req-4"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistSources(context.Background(), "proxy-1", []SourceInput{})
	if err == nil {
		t.Fatal("expected error when response has errors")
	}
	if result {
		t.Error("expected result to be false on error")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("expected error to contain first error message, got: %v", err)
	}
}

func TestPersistSources_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistSources(context.Background(), "proxy-1", []SourceInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if result {
		t.Error("expected false on error")
	}
}

func TestPersistSources_EmptySourcesList(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sSources": true, "errors": []},
		"request_id": "req-5"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistSources(context.Background(), "proxy-1", []SourceInput{})
	if err != nil {
		t.Fatalf("PersistSources returned error: %v", err)
	}
	if !result {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// PersistNamespaceSources tests
// ---------------------------------------------------------------------------

func TestPersistNamespaceSources_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sNamespaces": true, "errors": []},
		"request_id": "req-ns-1"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistNamespaceSources(context.Background(), "proxy-1", []NamespaceInput{
		{
			CurrentStreamName: "stream-1",
			Namespace:         "default",
			Selected:          true,
		},
	})
	if err != nil {
		t.Fatalf("PersistNamespaceSources returned error: %v", err)
	}
	if !result {
		t.Error("expected result to be true")
	}
}

func TestPersistNamespaceSources_False(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sNamespaces": false, "errors": []},
		"request_id": "req-ns-2"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistNamespaceSources(context.Background(), "proxy-1", nil)
	if err != nil {
		t.Fatalf("PersistNamespaceSources returned error: %v", err)
	}
	if result {
		t.Error("expected result to be false")
	}
}

func TestPersistNamespaceSources_WithErrors(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"persistK8sNamespaces": false,
			"errors": [{"message": "namespace not found"}, {"message": "permission denied"}]
		},
		"request_id": "req-ns-3"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistNamespaceSources(context.Background(), "proxy-1", []NamespaceInput{})
	if err == nil {
		t.Fatal("expected error when response has errors")
	}
	if result {
		t.Error("expected result to be false on error")
	}
	if !strings.Contains(err.Error(), "namespace not found") {
		t.Errorf("expected error to contain first error message, got: %v", err)
	}
}

func TestPersistNamespaceSources_ExecuteError(t *testing.T) {
	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, authSuccessBody())
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistNamespaceSources(context.Background(), "proxy-1", []NamespaceInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if result {
		t.Error("expected false on error")
	}
}

func TestPersistNamespaceSources_EmptyList(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sNamespaces": true, "errors": []},
		"request_id": "req-ns-4"
	}`

	callCount := 0
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			fmt.Fprint(w, authSuccessBody())
		} else {
			fmt.Fprint(w, remoteFetchResponse(inner))
		}
	})
	defer ts.Close()

	client, _ := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	result, err := client.PersistNamespaceSources(context.Background(), "proxy-1", []NamespaceInput{})
	if err != nil {
		t.Fatalf("PersistNamespaceSources returned error: %v", err)
	}
	if !result {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// NewClient with logger options tests
// ---------------------------------------------------------------------------

func TestNewClient_WithCustomLogger(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	ml := &noopLogger{}
	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	}, logger.WithLogger(ml))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.logger != ml {
		t.Error("expected custom logger to be set on client")
	}
}

func TestNewClient_WithoutLoggerUsesDefault(t *testing.T) {
	ts := newTestServer(http.StatusOK, authSuccessBody())
	defer ts.Close()

	client, err := NewClient(context.Background(), ClientConfig{
		Hostname: trimScheme(ts.URL),
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.logger == nil {
		t.Fatal("expected non-nil default logger")
	}
}

// ---------------------------------------------------------------------------
// sanitizeBody tests
// ---------------------------------------------------------------------------

func TestSanitizeBody_NoSensitiveKeys(t *testing.T) {
	req := graphQLRequest{
		Query:     "query { hello }",
		Variables: map[string]interface{}{"name": "alice", "count": 42},
	}
	result := sanitizeBody(req)

	var decoded graphQLRequest
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Variables["name"] != "alice" {
		t.Errorf("expected name='alice', got %v", decoded.Variables["name"])
	}
	// JSON numbers decode as float64
	if decoded.Variables["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", decoded.Variables["count"])
	}
}

func TestSanitizeBody_PasswordMasked(t *testing.T) {
	req := graphQLRequest{
		Query:     SignInMutation,
		Variables: map[string]interface{}{"email": "user@test.com", "password": "s3cret!"},
	}
	result := sanitizeBody(req)

	if strings.Contains(result, "s3cret!") {
		t.Error("expected password to be masked, but found plaintext")
	}
	if !strings.Contains(result, "****") {
		t.Error("expected masked password '****' in output")
	}
	// email should NOT be masked
	if !strings.Contains(result, "user@test.com") {
		t.Error("expected email to remain unmasked")
	}
}

func TestSanitizeBody_PasswordCaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"lowercase", "password"},
		{"uppercase", "Password"},
		{"all caps", "PASSWORD"},
		{"mixed case", "PaSsWoRd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := graphQLRequest{
				Query:     "q",
				Variables: map[string]interface{}{tt.key: "secret123"},
			}
			result := sanitizeBody(req)
			if strings.Contains(result, "secret123") {
				t.Errorf("expected %q key to be masked, but found plaintext", tt.key)
			}
			if !strings.Contains(result, "****") {
				t.Errorf("expected masked value for %q", tt.key)
			}
		})
	}
}

func TestSanitizeBody_NilVariables(t *testing.T) {
	req := graphQLRequest{
		Query:     "query { hello }",
		Variables: nil,
	}
	result := sanitizeBody(req)

	var decoded graphQLRequest
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(decoded.Variables) != 0 {
		t.Errorf("expected empty variables, got %v", decoded.Variables)
	}
}

func TestSanitizeBody_EmptyVariables(t *testing.T) {
	req := graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{},
	}
	result := sanitizeBody(req)

	var decoded graphQLRequest
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(decoded.Variables) != 0 {
		t.Errorf("expected empty variables, got %v", decoded.Variables)
	}
}

func TestSanitizeBody_PreservesQuery(t *testing.T) {
	req := graphQLRequest{
		Query:     "mutation doThing { field }",
		Variables: map[string]interface{}{"password": "hidden"},
	}
	result := sanitizeBody(req)

	var decoded graphQLRequest
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Query != req.Query {
		t.Errorf("expected query preserved, got %q", decoded.Query)
	}
}

func TestSanitizeBody_MixedSensitiveAndNormal(t *testing.T) {
	req := graphQLRequest{
		Query: "q",
		Variables: map[string]interface{}{
			"email":    "a@b.com",
			"password": "topsecret",
			"name":     "test",
		},
	}
	result := sanitizeBody(req)

	if strings.Contains(result, "topsecret") {
		t.Error("password should be masked")
	}
	if !strings.Contains(result, "a@b.com") {
		t.Error("email should not be masked")
	}
	if !strings.Contains(result, "test") {
		t.Error("name should not be masked")
	}
}

func TestSanitizeBody_DoesNotMutateOriginal(t *testing.T) {
	req := graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{"password": "original"},
	}
	_ = sanitizeBody(req)

	// Original should be untouched
	if req.Variables["password"] != "original" {
		t.Errorf("sanitizeBody mutated the original request; password=%v", req.Variables["password"])
	}
}

func TestSanitizeBody_NonStringPassword(t *testing.T) {
	req := graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{"password": 12345},
	}
	result := sanitizeBody(req)

	// Even numeric password values should be masked
	if strings.Contains(result, "12345") {
		t.Error("expected numeric password to be masked")
	}
	if !strings.Contains(result, "****") {
		t.Error("expected masked value")
	}
}

func TestSanitizeBody_MarshalError(t *testing.T) {
	// A channel cannot be marshaled to JSON, triggering the error path
	req := graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{"ch": make(chan int)},
	}
	result := sanitizeBody(req)

	expected := "<failed to marshal sanitized body>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeBody_ReturnsValidJSON(t *testing.T) {
	req := graphQLRequest{
		Query: "q",
		Variables: map[string]interface{}{
			"password": "secret",
			"nested":   map[string]interface{}{"key": "value"},
			"list":     []interface{}{1, 2, 3},
		},
	}
	result := sanitizeBody(req)

	if !json.Valid([]byte(result)) {
		t.Errorf("sanitizeBody returned invalid JSON: %s", result)
	}
}

// ---------------------------------------------------------------------------
// sensitiveKeys tests
// ---------------------------------------------------------------------------

func TestSensitiveKeys_ContainsPassword(t *testing.T) {
	if !sensitiveKeys["password"] {
		t.Error("expected 'password' to be in sensitiveKeys")
	}
}

func TestSensitiveKeys_DoesNotContainEmail(t *testing.T) {
	if sensitiveKeys["email"] {
		t.Error("did not expect 'email' to be in sensitiveKeys")
	}
}
