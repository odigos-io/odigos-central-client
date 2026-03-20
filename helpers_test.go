package odigos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch tests
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_Success(t *testing.T) {
	type testPayload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	inner := `{"name":"hello","count":42}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[testPayload]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if result.Name != "hello" {
		t.Errorf("expected name='hello', got %q", result.Name)
	}
	if result.Count != 42 {
		t.Errorf("expected count=42, got %d", result.Count)
	}
}

func TestUnmarshalRemoteFetch_NestedStruct(t *testing.T) {
	type inner struct {
		Value string `json:"value"`
	}
	type outer struct {
		Items []inner `json:"items"`
	}

	payload := `{"items":[{"value":"a"},{"value":"b"}]}`
	escaped, _ := json.Marshal(payload)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[outer]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Value != "a" {
		t.Errorf("expected first item='a', got %q", result.Items[0].Value)
	}
}

func TestUnmarshalRemoteFetch_InvalidOuterJSON(t *testing.T) {
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid outer JSON")
	}
}

func TestUnmarshalRemoteFetch_EmptyInput(t *testing.T) {
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(``))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestUnmarshalRemoteFetch_NullRemoteFetch(t *testing.T) {
	raw := `{"data":{"remoteFetch":null}}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for null remoteFetch")
	}
}

func TestUnmarshalRemoteFetch_RemoteFetchNotString(t *testing.T) {
	// remoteFetch is expected to be a JSON string, not a raw object
	raw := `{"data":{"remoteFetch":{"key":"value"}}}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error when remoteFetch is not a JSON string")
	}
}

func TestUnmarshalRemoteFetch_InvalidInnerJSON(t *testing.T) {
	// Inner string is not valid JSON
	escaped, _ := json.Marshal("not-json{{{")
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	_, err := UnmarshalRemoteFetch[struct{ Name string }]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for invalid inner JSON")
	}
}

func TestUnmarshalRemoteFetch_TypeMismatch(t *testing.T) {
	type expected struct {
		Count int `json:"count"`
	}
	// Inner JSON has count as a string instead of int
	inner := `{"count":"not-a-number"}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	_, err := UnmarshalRemoteFetch[expected]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestUnmarshalRemoteFetch_EmptyInnerObject(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	inner := `{}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[payload]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if result.Name != "" {
		t.Errorf("expected empty name, got %q", result.Name)
	}
}

func TestUnmarshalRemoteFetch_MissingDataField(t *testing.T) {
	// data field exists but remoteFetch is missing
	raw := `{"data":{}}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error when remoteFetch is missing")
	}
}

func TestUnmarshalRemoteFetch_WithNamespacesResponse(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"k8sActualNamespaces": [
					{"name": "ns1", "selected": true, "dataStreamNames": []},
					{"name": "ns2", "selected": false, "dataStreamNames": []}
				]
			}
		},
		"request_id": "req-abc"
	}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[namespacesResponse]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if result.Type != "data" {
		t.Errorf("expected type='data', got %q", result.Type)
	}
	if result.RequestID != "req-abc" {
		t.Errorf("expected requestID='req-abc', got %q", result.RequestID)
	}
	if len(result.Data.ComputePlatform.K8SActualNamespaces) != 2 {
		t.Fatalf("expected 2 namespaces, got %d", len(result.Data.ComputePlatform.K8SActualNamespaces))
	}
}

func TestUnmarshalRemoteFetch_WithSourcesResponse(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"sources": [
					{
						"name": "svc-a",
						"namespace": "prod",
						"kind": "Deployment",
						"dataStreamNames": [],
						"selected": true,
						"otelServiceName": "svc-a",
						"containers": [],
						"conditions": []
					}
				]
			}
		},
		"request_id": "req-xyz"
	}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[sourcesResponse]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if len(result.Data.ComputePlatform.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(result.Data.ComputePlatform.Sources))
	}
	if result.Data.ComputePlatform.Sources[0].Name != "svc-a" {
		t.Errorf("expected source name='svc-a', got %q", result.Data.ComputePlatform.Sources[0].Name)
	}
}

func TestUnmarshalRemoteFetch_WithPersistSourcesResponse(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sSources": true, "errors": []},
		"request_id": "req-persist"
	}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[persistSourcesResponse]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if !result.Data.PersistK8sSources {
		t.Error("expected persistK8sSources=true")
	}
	if len(result.Data.Errors) != 0 {
		t.Errorf("expected no errors, got %d", len(result.Data.Errors))
	}
}

// ---------------------------------------------------------------------------
// fetchRemote tests
// ---------------------------------------------------------------------------

func TestFetchRemote_Success(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"computePlatform": {"k8sActualNamespaces": [{"name":"ns","selected":true,"dataStreamNames":[]}]}},
		"request_id": "r"
	}`
	escaped, _ := json.Marshal(inner)
	resp := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	ts := newTestServer(http.StatusOK, resp)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "tok")

	result, err := fetchRemote[namespacesResponse](context.Background(), client, "proxy-1", GetNamespacesQuery, map[string]any{})
	if err != nil {
		t.Fatalf("fetchRemote returned error: %v", err)
	}
	if len(result.Data.ComputePlatform.K8SActualNamespaces) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(result.Data.ComputePlatform.K8SActualNamespaces))
	}
	if result.Data.ComputePlatform.K8SActualNamespaces[0].Name != "ns" {
		t.Errorf("expected namespace name='ns', got %q", result.Data.ComputePlatform.K8SActualNamespaces[0].Name)
	}
}

func TestFetchRemote_UsesRemoteFetchQuery(t *testing.T) {
	var receivedBody []byte
	ts := newTestServerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = readAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		inner := `{"type":"data","data":{"computePlatform":{"k8sActualNamespaces":[]}},"request_id":"r"}`
		escaped, _ := json.Marshal(inner)
		fmt.Fprintf(w, `{"data":{"remoteFetch":%s}}`, string(escaped))
	})
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "tok")

	_, err := fetchRemote[namespacesResponse](context.Background(), client, "my-proxy", GetNamespacesQuery, map[string]any{"key": "val"})
	if err != nil {
		t.Fatalf("fetchRemote returned error: %v", err)
	}

	var req graphQLRequest
	if err := json.Unmarshal(receivedBody, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	// Should use the RemoteFetchQuery wrapper, not the inner query
	if req.Query != RemoteFetchQuery {
		t.Errorf("expected query to be RemoteFetchQuery, got %q", req.Query)
	}
	if req.Variables["proxyID"] != "my-proxy" {
		t.Errorf("expected proxyID='my-proxy', got %v", req.Variables["proxyID"])
	}
	if req.Variables["query"] != GetNamespacesQuery {
		t.Errorf("expected inner query to be GetNamespacesQuery")
	}
	vars, ok := req.Variables["variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected variables to be a map")
	}
	if vars["key"] != "val" {
		t.Errorf("expected key='val', got %v", vars["key"])
	}
}

func TestFetchRemote_ExecuteError(t *testing.T) {
	ts := newTestServer(http.StatusInternalServerError, `error`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")

	_, err := fetchRemote[namespacesResponse](context.Background(), client, "proxy-1", GetNamespacesQuery, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchRemote_UnmarshalError(t *testing.T) {
	ts := newTestServer(http.StatusOK, `{"data":{"remoteFetch": 12345}}`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")

	_, err := fetchRemote[namespacesResponse](context.Background(), client, "proxy-1", GetNamespacesQuery, nil)
	if err == nil {
		t.Fatal("expected error for invalid remoteFetch value")
	}
}

func TestFetchRemote_CancelledContext(t *testing.T) {
	ts := newTestServer(http.StatusOK, `{}`)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchRemote[namespacesResponse](ctx, client, "proxy-1", GetNamespacesQuery, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchRemote_NilVariables(t *testing.T) {
	inner := `{
		"type":"data",
		"data":{"computePlatform":{"k8sActualNamespaces":[]}},
		"request_id":"r"
	}`
	escaped, _ := json.Marshal(inner)
	resp := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	ts := newTestServer(http.StatusOK, resp)
	defer ts.Close()

	client := newTestClient(ts.URL, ts.Client(), "")

	result, err := fetchRemote[namespacesResponse](context.Background(), client, "proxy-1", GetNamespacesQuery, nil)
	if err != nil {
		t.Fatalf("fetchRemote returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch – outer GraphQL errors
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_OuterGraphQLErrors(t *testing.T) {
	raw := `{"data":{"remoteFetch":null},"errors":[{"message":"unauthorized"}]}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for outer GraphQL errors")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected error to contain 'unauthorized', got: %v", err)
	}
	if !strings.Contains(err.Error(), "GraphQL errors") {
		t.Errorf("expected error to mention 'GraphQL errors', got: %v", err)
	}
}

func TestUnmarshalRemoteFetch_MultipleOuterGraphQLErrors(t *testing.T) {
	raw := `{"data":{"remoteFetch":null},"errors":[{"message":"err-a"},{"message":"err-b"},{"message":"err-c"}]}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for multiple outer errors")
	}
	if !strings.Contains(err.Error(), "err-a; err-b; err-c") {
		t.Errorf("expected all error messages joined with '; ', got: %v", err)
	}
}

func TestUnmarshalRemoteFetch_SingleOuterError(t *testing.T) {
	raw := `{"data":{"remoteFetch":"{}"},"errors":[{"message":"single error"}]}`
	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "single error") {
		t.Errorf("expected 'single error', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch – empty remoteFetch string
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_EmptyRemoteFetchString(t *testing.T) {
	escaped, _ := json.Marshal("")
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for empty remoteFetch string")
	}
	if !strings.Contains(err.Error(), "empty data string") {
		t.Errorf("expected 'empty data string' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch – inner remote GraphQL errors via checkRemoteGraphQLErrors
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_InnerRemoteGraphQLErrors(t *testing.T) {
	inner := `{"type":"graphql_response","data":{"errors":[{"message":"field not found"}]},"request_id":"r"}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	_, err := UnmarshalRemoteFetch[struct{}]([]byte(raw))
	if err == nil {
		t.Fatal("expected error for inner GraphQL errors")
	}
	if !strings.Contains(err.Error(), "remote GraphQL errors") {
		t.Errorf("expected 'remote GraphQL errors', got: %v", err)
	}
	if !strings.Contains(err.Error(), "field not found") {
		t.Errorf("expected 'field not found', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch – with persistNamespaceSourcesResponse
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_WithPersistNamespaceSourcesResponse(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {"persistK8sNamespaces": true, "errors": []},
		"request_id": "req-ns-persist"
	}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[persistNamespaceSourcesResponse]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if !result.Data.PersistK8sNamespaces {
		t.Error("expected persistK8sNamespaces=true")
	}
	if len(result.Data.Errors) != 0 {
		t.Errorf("expected no errors, got %d", len(result.Data.Errors))
	}
	if result.RequestID != "req-ns-persist" {
		t.Errorf("expected request_id='req-ns-persist', got %q", result.RequestID)
	}
}

// ---------------------------------------------------------------------------
// UnmarshalRemoteFetch – with sourceResponse (single source)
// ---------------------------------------------------------------------------

func TestUnmarshalRemoteFetch_WithSourceResponse(t *testing.T) {
	inner := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"source": {
					"name": "api-gw",
					"namespace": "infra",
					"kind": "Deployment",
					"dataStreamNames": ["ds-1"],
					"selected": true,
					"otelServiceName": "api-gw-otel",
					"containers": [],
					"conditions": []
				}
			}
		},
		"request_id": "req-src"
	}`
	escaped, _ := json.Marshal(inner)
	raw := fmt.Sprintf(`{"data":{"remoteFetch":%s}}`, string(escaped))

	result, err := UnmarshalRemoteFetch[sourceResponse]([]byte(raw))
	if err != nil {
		t.Fatalf("UnmarshalRemoteFetch returned error: %v", err)
	}
	if result.Data.ComputePlatform.Source.Name != "api-gw" {
		t.Errorf("expected name='api-gw', got %q", result.Data.ComputePlatform.Source.Name)
	}
}

// ---------------------------------------------------------------------------
// checkRemoteGraphQLErrors – direct unit tests
// ---------------------------------------------------------------------------

func TestCheckRemoteGraphQLErrors_NoErrors(t *testing.T) {
	inner := `{"type":"graphql_response","data":{"computePlatform":{}},"request_id":"r"}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_EmptyErrorsArray(t *testing.T) {
	inner := `{"data":{"errors":[]}}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err != nil {
		t.Fatalf("expected nil error for empty errors array, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_SingleError(t *testing.T) {
	inner := `{"data":{"errors":[{"message":"access denied"}]}}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "remote GraphQL errors") {
		t.Errorf("expected 'remote GraphQL errors' prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected 'access denied', got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_MultipleErrors(t *testing.T) {
	inner := `{"data":{"errors":[{"message":"err1"},{"message":"err2"},{"message":"err3"}]}}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "err1; err2; err3") {
		t.Errorf("expected joined messages, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_InvalidJSON(t *testing.T) {
	err := checkRemoteGraphQLErrors([]byte(`not json at all`))
	if err != nil {
		t.Fatalf("expected nil error for unparsable input, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_NoDataField(t *testing.T) {
	inner := `{"type":"something"}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err != nil {
		t.Fatalf("expected nil error when data field is missing, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_EmptyInput(t *testing.T) {
	err := checkRemoteGraphQLErrors([]byte(`{}`))
	if err != nil {
		t.Fatalf("expected nil error for empty object, got: %v", err)
	}
}

func TestCheckRemoteGraphQLErrors_ErrorWithEmptyMessage(t *testing.T) {
	inner := `{"data":{"errors":[{"message":""}]}}`
	err := checkRemoteGraphQLErrors([]byte(inner))
	if err == nil {
		t.Fatal("expected error even with empty message")
	}
}

// readAll is a small helper to avoid importing io in this file again.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
