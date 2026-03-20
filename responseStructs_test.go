package odigos

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// graphQLError tests
// ---------------------------------------------------------------------------

func TestGraphQLError_JSON(t *testing.T) {
	raw := `{"message":"something went wrong"}`
	var e graphQLError
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if e.Message != "something went wrong" {
		t.Errorf("expected message='something went wrong', got %q", e.Message)
	}

	// Round-trip
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var e2 graphQLError
	if err := json.Unmarshal(b, &e2); err != nil {
		t.Fatalf("Unmarshal round-trip error: %v", err)
	}
	if e2.Message != e.Message {
		t.Errorf("round-trip mismatch: %q != %q", e2.Message, e.Message)
	}
}

func TestGraphQLError_EmptyMessage(t *testing.T) {
	raw := `{"message":""}`
	var e graphQLError
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if e.Message != "" {
		t.Errorf("expected empty message, got %q", e.Message)
	}
}

func TestGraphQLError_MissingField(t *testing.T) {
	raw := `{}`
	var e graphQLError
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if e.Message != "" {
		t.Errorf("expected zero-value message, got %q", e.Message)
	}
}

// ---------------------------------------------------------------------------
// authResponse tests
// ---------------------------------------------------------------------------

func TestAuthResponse_FullPayload(t *testing.T) {
	raw := `{
		"data": {
			"signIn": {
				"accessToken": "abc123",
				"idToken": "id-tok",
				"status": "SUCCESS",
				"user": {
					"id": "u-1",
					"email": "test@test.com",
					"username": "tester",
					"role": "admin",
					"needsPasswordChange": true,
					"teams": [],
					"computePlatforms": [
						{"id":"cp1","name":"prod","odigosVersion":"1.0","type":"K8S","status":"CONNECTED","connectedAt":100,"lastSeenAt":200,"users":[],"teams":[],"__typename":"CP"}
					],
					"__typename": "User"
				},
				"__typename": "AuthPayload"
			}
		},
		"errors": []
	}`
	var resp authResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.SignIn == nil {
		t.Fatal("expected non-nil SignIn")
	}
	if resp.Data.SignIn.AccessToken != "abc123" {
		t.Errorf("expected accessToken='abc123', got %q", resp.Data.SignIn.AccessToken)
	}
	if resp.Data.SignIn.IDToken != "id-tok" {
		t.Errorf("expected idToken='id-tok', got %q", resp.Data.SignIn.IDToken)
	}
	if resp.Data.SignIn.Status != "SUCCESS" {
		t.Errorf("expected status='SUCCESS', got %q", resp.Data.SignIn.Status)
	}
	if resp.Data.SignIn.User.Email != "test@test.com" {
		t.Errorf("expected email='test@test.com', got %q", resp.Data.SignIn.User.Email)
	}
	if !resp.Data.SignIn.User.NeedsPasswordChange {
		t.Error("expected needsPasswordChange=true")
	}
	if len(resp.Data.SignIn.User.ComputePlatforms) != 1 {
		t.Fatalf("expected 1 compute platform, got %d", len(resp.Data.SignIn.User.ComputePlatforms))
	}
	if len(resp.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(resp.Errors))
	}
}

func TestAuthResponse_NullSignIn(t *testing.T) {
	raw := `{"data": {"signIn": null}, "errors": [{"message": "bad"}]}`
	var resp authResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.SignIn != nil {
		t.Error("expected nil SignIn")
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(resp.Errors))
	}
	if resp.Errors[0].Message != "bad" {
		t.Errorf("expected message='bad', got %q", resp.Errors[0].Message)
	}
}

func TestAuthResponse_EmptyData(t *testing.T) {
	raw := `{"data": {}}`
	var resp authResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.SignIn != nil {
		t.Error("expected nil SignIn for empty data")
	}
}

// ---------------------------------------------------------------------------
// ComputePlatform tests
// ---------------------------------------------------------------------------

func TestComputePlatform_JSON(t *testing.T) {
	raw := `{
		"id": "cp-1",
		"name": "my-cluster",
		"odigosVersion": "2.0.0",
		"type": "K8S",
		"status": "CONNECTED",
		"connectedAt": 1700000000,
		"lastSeenAt": 1700000100,
		"users": [{"id": "u1"}],
		"teams": [],
		"__typename": "ComputePlatform"
	}`
	var cp ComputePlatform
	if err := json.Unmarshal([]byte(raw), &cp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if cp.ID != "cp-1" {
		t.Errorf("expected id='cp-1', got %q", cp.ID)
	}
	if cp.Name != "my-cluster" {
		t.Errorf("expected name='my-cluster', got %q", cp.Name)
	}
	if cp.OdigosVersion != "2.0.0" {
		t.Errorf("expected odigosVersion='2.0.0', got %q", cp.OdigosVersion)
	}
	if cp.ConnectedAt != 1700000000 {
		t.Errorf("expected connectedAt=1700000000, got %d", cp.ConnectedAt)
	}
	if cp.Typename != "ComputePlatform" {
		t.Errorf("expected __typename='ComputePlatform', got %q", cp.Typename)
	}
	if len(cp.Users) != 1 {
		t.Errorf("expected 1 user, got %d", len(cp.Users))
	}
}

func TestComputePlatform_ZeroValues(t *testing.T) {
	raw := `{}`
	var cp ComputePlatform
	if err := json.Unmarshal([]byte(raw), &cp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if cp.ID != "" || cp.Name != "" || cp.Status != "" {
		t.Error("expected zero values for empty JSON")
	}
}

func TestComputePlatform_RoundTrip(t *testing.T) {
	original := ComputePlatform{
		ID:            "cp-99",
		Name:          "test",
		OdigosVersion: "3.0",
		Type:          "K8S",
		Status:        "ACTIVE",
		ConnectedAt:   1234567890,
		LastSeenAt:    1234567999,
		Typename:      "CP",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded ComputePlatform
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.ID != original.ID || decoded.Name != original.Name || decoded.Status != original.Status ||
		decoded.ConnectedAt != original.ConnectedAt || decoded.LastSeenAt != original.LastSeenAt ||
		decoded.OdigosVersion != original.OdigosVersion || decoded.Type != original.Type {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
}

// ---------------------------------------------------------------------------
// computePlatformsResponse tests
// ---------------------------------------------------------------------------

func TestComputePlatformsResponse_JSON(t *testing.T) {
	raw := `{
		"data": {
			"computePlatforms": [
				{"id": "cp-1", "name": "a", "odigosVersion": "1.0", "type": "K8S", "status": "OK", "connectedAt": 0, "lastSeenAt": 0, "users": [], "teams": [], "__typename": "CP"},
				{"id": "cp-2", "name": "b", "odigosVersion": "1.1", "type": "K8S", "status": "OK", "connectedAt": 0, "lastSeenAt": 0, "users": [], "teams": [], "__typename": "CP"}
			]
		}
	}`
	var resp computePlatformsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(resp.Data.ComputePlatforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(resp.Data.ComputePlatforms))
	}
}

func TestComputePlatformsResponse_EmptyList(t *testing.T) {
	raw := `{"data": {"computePlatforms": []}}`
	var resp computePlatformsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(resp.Data.ComputePlatforms) != 0 {
		t.Errorf("expected 0 platforms, got %d", len(resp.Data.ComputePlatforms))
	}
}

func TestComputePlatformsResponse_NullList(t *testing.T) {
	raw := `{"data": {"computePlatforms": null}}`
	var resp computePlatformsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.ComputePlatforms != nil {
		t.Errorf("expected nil platforms, got %v", resp.Data.ComputePlatforms)
	}
}

// ---------------------------------------------------------------------------
// GraphQLRemoteResponse tests
// ---------------------------------------------------------------------------

func TestGraphQLRemoteResponse_JSON(t *testing.T) {
	raw := `{"data":{"remoteFetch":"some string content"}}`
	var resp GraphQLRemoteResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	var s string
	if err := json.Unmarshal(resp.Data.RemoteFetch, &s); err != nil {
		t.Fatalf("Unmarshal remoteFetch: %v", err)
	}
	if s != "some string content" {
		t.Errorf("expected 'some string content', got %q", s)
	}
}

func TestGraphQLRemoteResponse_NullRemoteFetch(t *testing.T) {
	raw := `{"data":{"remoteFetch":null}}`
	var resp GraphQLRemoteResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if string(resp.Data.RemoteFetch) != "null" {
		t.Errorf("expected null raw message, got %s", resp.Data.RemoteFetch)
	}
}

func TestGraphQLRemoteResponse_ObjectRemoteFetch(t *testing.T) {
	raw := `{"data":{"remoteFetch":{"key":"value"}}}`
	var resp GraphQLRemoteResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	// Should be valid raw JSON
	if !json.Valid(resp.Data.RemoteFetch) {
		t.Error("expected valid raw JSON in remoteFetch")
	}
}

// ---------------------------------------------------------------------------
// K8sActualNamespace tests
// ---------------------------------------------------------------------------

func TestK8sActualNamespace_JSON(t *testing.T) {
	raw := `{"name":"production","selected":true,"dataStreamNames":["stream1","stream2"]}`
	var ns K8sActualNamespace
	if err := json.Unmarshal([]byte(raw), &ns); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if ns.Name != "production" {
		t.Errorf("expected name='production', got %q", ns.Name)
	}
	if !ns.Selected {
		t.Error("expected selected=true")
	}
	if len(ns.DataStreamNames) != 2 {
		t.Errorf("expected 2 data stream names, got %d", len(ns.DataStreamNames))
	}
}

func TestK8sActualNamespace_ZeroValues(t *testing.T) {
	raw := `{}`
	var ns K8sActualNamespace
	if err := json.Unmarshal([]byte(raw), &ns); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if ns.Name != "" {
		t.Errorf("expected empty name, got %q", ns.Name)
	}
	if ns.Selected {
		t.Error("expected selected=false")
	}
}

func TestK8sActualNamespace_RoundTrip(t *testing.T) {
	original := K8sActualNamespace{
		Name:     "kube-system",
		Selected: false,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded K8sActualNamespace
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Name != original.Name || decoded.Selected != original.Selected {
		t.Errorf("round-trip mismatch")
	}
}

// ---------------------------------------------------------------------------
// namespacesResponse tests
// ---------------------------------------------------------------------------

func TestNamespacesResponse_JSON(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"k8sActualNamespaces": [
					{"name": "ns1", "selected": false, "dataStreamNames": null}
				]
			}
		},
		"request_id": "abc-123"
	}`
	var resp namespacesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Type != "data" {
		t.Errorf("expected type='data', got %q", resp.Type)
	}
	if resp.RequestID != "abc-123" {
		t.Errorf("expected request_id='abc-123', got %q", resp.RequestID)
	}
	if len(resp.Data.ComputePlatform.K8SActualNamespaces) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(resp.Data.ComputePlatform.K8SActualNamespaces))
	}
}

// ---------------------------------------------------------------------------
// SourceCondition tests
// ---------------------------------------------------------------------------

func TestSourceCondition_JSON(t *testing.T) {
	raw := `{
		"lastTransitionTime": "2024-06-15T10:30:00Z",
		"message": "instrumentation complete",
		"reason": "InstrumentationReady",
		"status": "True",
		"type": "Ready"
	}`
	var sc SourceCondition
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sc.Status != "True" {
		t.Errorf("expected status='True', got %q", sc.Status)
	}
	if sc.Type != "Ready" {
		t.Errorf("expected type='Ready', got %q", sc.Type)
	}
	if sc.Reason != "InstrumentationReady" {
		t.Errorf("expected reason='InstrumentationReady', got %q", sc.Reason)
	}
	if sc.Message != "instrumentation complete" {
		t.Errorf("expected message='instrumentation complete', got %q", sc.Message)
	}
	expectedTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if !sc.LastTransitionTime.Equal(expectedTime) {
		t.Errorf("expected time=%v, got %v", expectedTime, sc.LastTransitionTime)
	}
}

func TestSourceCondition_ZeroValues(t *testing.T) {
	raw := `{}`
	var sc SourceCondition
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sc.Status != "" || sc.Type != "" || sc.Reason != "" || sc.Message != "" {
		t.Error("expected zero values")
	}
	if !sc.LastTransitionTime.IsZero() {
		t.Error("expected zero time")
	}
}

func TestSourceCondition_RoundTrip(t *testing.T) {
	original := SourceCondition{
		LastTransitionTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:            "msg",
		Reason:             "reason",
		Status:             "False",
		Type:               "Progressing",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded SourceCondition
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Status != original.Status || decoded.Type != original.Type {
		t.Error("round-trip mismatch")
	}
	if !decoded.LastTransitionTime.Equal(original.LastTransitionTime) {
		t.Errorf("time mismatch: %v != %v", decoded.LastTransitionTime, original.LastTransitionTime)
	}
}

// ---------------------------------------------------------------------------
// SourceContainer tests
// ---------------------------------------------------------------------------

func TestSourceContainer_JSON(t *testing.T) {
	raw := `{
		"containerName": "main",
		"instrumentationMessage": "ok",
		"instrumented": true,
		"language": "python",
		"otelDistroName": "odigos",
		"overriden": false,
		"runtimeVersion": "3.11"
	}`
	var sc SourceContainer
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sc.ContainerName != "main" {
		t.Errorf("expected containerName='main', got %q", sc.ContainerName)
	}
	if !sc.Instrumented {
		t.Error("expected instrumented=true")
	}
	if sc.Language != "python" {
		t.Errorf("expected language='python', got %q", sc.Language)
	}
	if sc.Overriden {
		t.Error("expected overriden=false")
	}
	if sc.RuntimeVersion != "3.11" {
		t.Errorf("expected runtimeVersion='3.11', got %q", sc.RuntimeVersion)
	}
}

func TestSourceContainer_ZeroValues(t *testing.T) {
	raw := `{}`
	var sc SourceContainer
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sc.ContainerName != "" || sc.Language != "" || sc.Instrumented || sc.Overriden {
		t.Error("expected zero values")
	}
}

func TestSourceContainer_RoundTrip(t *testing.T) {
	original := SourceContainer{
		ContainerName:          "sidecar",
		InstrumentationMessage: "waiting",
		Instrumented:           false,
		Language:               "java",
		OtelDistroName:         "custom",
		Overriden:              true,
		RuntimeVersion:         "17",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded SourceContainer
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
}

// ---------------------------------------------------------------------------
// Source tests
// ---------------------------------------------------------------------------

func TestSource_JSON(t *testing.T) {
	raw := `{
		"conditions": [
			{"lastTransitionTime": "2024-01-01T00:00:00Z", "message": "ready", "reason": "Done", "status": "True", "type": "Ready"}
		],
		"containers": [
			{"containerName": "app", "instrumentationMessage": "", "instrumented": true, "language": "go", "otelDistroName": "d", "overriden": false, "runtimeVersion": "1.21"}
		],
		"dataStreamNames": ["s1"],
		"kind": "Deployment",
		"name": "web-app",
		"namespace": "default",
		"otelServiceName": "web-app-svc",
		"selected": true
	}`
	var src Source
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if src.Name != "web-app" {
		t.Errorf("expected name='web-app', got %q", src.Name)
	}
	if src.Namespace != "default" {
		t.Errorf("expected namespace='default', got %q", src.Namespace)
	}
	if src.Kind != "Deployment" {
		t.Errorf("expected kind='Deployment', got %q", src.Kind)
	}
	if !src.Selected {
		t.Error("expected selected=true")
	}
	if len(src.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(src.Conditions))
	}
	if len(src.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(src.Containers))
	}
	if len(src.DataStreamNames) != 1 || src.DataStreamNames[0] != "s1" {
		t.Errorf("expected dataStreamNames=['s1'], got %v", src.DataStreamNames)
	}
}

func TestSource_ZeroValues(t *testing.T) {
	raw := `{}`
	var src Source
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if src.Name != "" || src.Kind != "" || src.Selected {
		t.Error("expected zero values")
	}
	if len(src.Conditions) != 0 || len(src.Containers) != 0 || len(src.DataStreamNames) != 0 {
		t.Error("expected empty slices")
	}
}

func TestSource_MultipleConditionsAndContainers(t *testing.T) {
	raw := `{
		"conditions": [
			{"status": "True", "type": "Ready", "reason": "", "message": "", "lastTransitionTime": "2024-01-01T00:00:00Z"},
			{"status": "False", "type": "Progressing", "reason": "err", "message": "fail", "lastTransitionTime": "2024-01-02T00:00:00Z"}
		],
		"containers": [
			{"containerName": "a", "language": "go", "instrumented": true, "instrumentationMessage": "", "otelDistroName": "", "overriden": false, "runtimeVersion": ""},
			{"containerName": "b", "language": "python", "instrumented": false, "instrumentationMessage": "pending", "otelDistroName": "", "overriden": false, "runtimeVersion": ""},
			{"containerName": "c", "language": "java", "instrumented": true, "instrumentationMessage": "", "otelDistroName": "", "overriden": true, "runtimeVersion": ""}
		],
		"dataStreamNames": ["s1", "s2", "s3"],
		"kind": "DaemonSet",
		"name": "multi",
		"namespace": "kube-system",
		"otelServiceName": "multi-svc",
		"selected": false
	}`
	var src Source
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(src.Conditions) != 2 {
		t.Errorf("expected 2 conditions, got %d", len(src.Conditions))
	}
	if len(src.Containers) != 3 {
		t.Errorf("expected 3 containers, got %d", len(src.Containers))
	}
	if len(src.DataStreamNames) != 3 {
		t.Errorf("expected 3 data stream names, got %d", len(src.DataStreamNames))
	}
}

// ---------------------------------------------------------------------------
// SourceInput tests
// ---------------------------------------------------------------------------

func TestSourceInput_JSON(t *testing.T) {
	raw := `{
		"currentStreamName": "stream-a",
		"kind": "Deployment",
		"name": "my-app",
		"namespace": "staging",
		"selected": true
	}`
	var si SourceInput
	if err := json.Unmarshal([]byte(raw), &si); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if si.CurrentStreamName != "stream-a" {
		t.Errorf("expected currentStreamName='stream-a', got %q", si.CurrentStreamName)
	}
	if si.Kind != "Deployment" {
		t.Errorf("expected kind='Deployment', got %q", si.Kind)
	}
	if si.Name != "my-app" {
		t.Errorf("expected name='my-app', got %q", si.Name)
	}
	if si.Namespace != "staging" {
		t.Errorf("expected namespace='staging', got %q", si.Namespace)
	}
	if !si.Selected {
		t.Error("expected selected=true")
	}
}

func TestSourceInput_RoundTrip(t *testing.T) {
	original := SourceInput{
		CurrentStreamName: "stream-b",
		Kind:              "StatefulSet",
		Name:              "db",
		Namespace:         "production",
		Selected:          false,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded SourceInput
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
}

func TestSourceInput_ZeroValues(t *testing.T) {
	raw := `{}`
	var si SourceInput
	if err := json.Unmarshal([]byte(raw), &si); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if si.CurrentStreamName != "" || si.Kind != "" || si.Name != "" || si.Namespace != "" || si.Selected {
		t.Error("expected zero values")
	}
}

// ---------------------------------------------------------------------------
// SourceId tests
// ---------------------------------------------------------------------------

func TestSourceId_JSON(t *testing.T) {
	raw := `{"kind":"Deployment","name":"my-app","namespace":"default"}`
	var sid SourceId
	if err := json.Unmarshal([]byte(raw), &sid); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sid.Kind != "Deployment" {
		t.Errorf("expected kind='Deployment', got %q", sid.Kind)
	}
	if sid.Name != "my-app" {
		t.Errorf("expected name='my-app', got %q", sid.Name)
	}
	if sid.Namespace != "default" {
		t.Errorf("expected namespace='default', got %q", sid.Namespace)
	}
}

func TestSourceId_RoundTrip(t *testing.T) {
	original := SourceId{Kind: "DaemonSet", Name: "agent", Namespace: "monitoring"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded SourceId
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
}

func TestSourceId_ZeroValues(t *testing.T) {
	raw := `{}`
	var sid SourceId
	if err := json.Unmarshal([]byte(raw), &sid); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if sid.Kind != "" || sid.Name != "" || sid.Namespace != "" {
		t.Error("expected zero values")
	}
}

// ---------------------------------------------------------------------------
// persistSourcesResponse tests
// ---------------------------------------------------------------------------

func TestPersistSourcesResponse_Success(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {"persistK8sSources": true, "errors": []},
		"request_id": "req-1"
	}`
	var resp persistSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !resp.Data.PersistK8sSources {
		t.Error("expected persistK8sSources=true")
	}
	if len(resp.Data.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(resp.Data.Errors))
	}
	if resp.RequestID != "req-1" {
		t.Errorf("expected request_id='req-1', got %q", resp.RequestID)
	}
}

func TestPersistSourcesResponse_WithErrors(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {
			"persistK8sSources": false,
			"errors": [{"message": "not found"}, {"message": "conflict"}]
		},
		"request_id": "req-2"
	}`
	var resp persistSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.PersistK8sSources {
		t.Error("expected persistK8sSources=false")
	}
	if len(resp.Data.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(resp.Data.Errors))
	}
	if resp.Data.Errors[0].Message != "not found" {
		t.Errorf("expected first error='not found', got %q", resp.Data.Errors[0].Message)
	}
	if resp.Data.Errors[1].Message != "conflict" {
		t.Errorf("expected second error='conflict', got %q", resp.Data.Errors[1].Message)
	}
}

func TestPersistSourcesResponse_NullErrors(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {"persistK8sSources": true, "errors": null},
		"request_id": "req-3"
	}`
	var resp persistSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.Errors != nil {
		t.Errorf("expected nil errors, got %v", resp.Data.Errors)
	}
}

// ---------------------------------------------------------------------------
// sourcesResponse tests
// ---------------------------------------------------------------------------

func TestSourcesResponse_JSON(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"sources": [
					{
						"name": "svc",
						"namespace": "ns",
						"kind": "Deployment",
						"dataStreamNames": [],
						"selected": true,
						"otelServiceName": "otel-svc",
						"containers": [],
						"conditions": []
					}
				]
			}
		},
		"request_id": "r-1"
	}`
	var resp sourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Type != "data" {
		t.Errorf("expected type='data', got %q", resp.Type)
	}
	if len(resp.Data.ComputePlatform.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(resp.Data.ComputePlatform.Sources))
	}
	if resp.Data.ComputePlatform.Sources[0].Name != "svc" {
		t.Errorf("expected source name='svc', got %q", resp.Data.ComputePlatform.Sources[0].Name)
	}
}

// ---------------------------------------------------------------------------
// sourceResponse tests
// ---------------------------------------------------------------------------

func TestSourceResponse_JSON(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {
			"computePlatform": {
				"source": {
					"name": "svc-single",
					"namespace": "prod",
					"kind": "StatefulSet",
					"dataStreamNames": ["ds1"],
					"selected": false,
					"otelServiceName": "single-otel",
					"containers": [],
					"conditions": []
				}
			}
		},
		"request_id": "r-single"
	}`
	var resp sourceResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.ComputePlatform.Source.Name != "svc-single" {
		t.Errorf("expected source name='svc-single', got %q", resp.Data.ComputePlatform.Source.Name)
	}
	if resp.Data.ComputePlatform.Source.Kind != "StatefulSet" {
		t.Errorf("expected kind='StatefulSet', got %q", resp.Data.ComputePlatform.Source.Kind)
	}
	if resp.RequestID != "r-single" {
		t.Errorf("expected request_id='r-single', got %q", resp.RequestID)
	}
}

// ---------------------------------------------------------------------------
// graphQLRequest tests (JSON serialization)
// ---------------------------------------------------------------------------

func TestGraphQLRequest_Marshal(t *testing.T) {
	req := graphQLRequest{
		Query:     "query { hello }",
		Variables: map[string]interface{}{"key": "value", "num": float64(42)},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded["query"] != "query { hello }" {
		t.Errorf("expected query='query { hello }', got %v", decoded["query"])
	}
	vars, ok := decoded["variables"].(map[string]interface{})
	if !ok {
		t.Fatal("expected variables to be a map")
	}
	if vars["key"] != "value" {
		t.Errorf("expected key='value', got %v", vars["key"])
	}
}

func TestGraphQLRequest_NilVariables(t *testing.T) {
	req := graphQLRequest{
		Query:     "q",
		Variables: nil,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	// nil map marshals to null in JSON
	if decoded["variables"] != nil {
		t.Errorf("expected null variables, got %v", decoded["variables"])
	}
}

func TestGraphQLRequest_EmptyVariables(t *testing.T) {
	req := graphQLRequest{
		Query:     "q",
		Variables: map[string]interface{}{},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	vars, ok := decoded["variables"].(map[string]interface{})
	if !ok {
		t.Fatal("expected variables to be a map")
	}
	if len(vars) != 0 {
		t.Errorf("expected empty variables map, got %v", vars)
	}
}

// ---------------------------------------------------------------------------
// persistNamespaceSourcesResponse tests
// ---------------------------------------------------------------------------

func TestPersistNamespaceSourcesResponse_Success(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {"persistK8sNamespaces": true, "errors": []},
		"request_id": "req-ns-1"
	}`
	var resp persistNamespaceSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !resp.Data.PersistK8sNamespaces {
		t.Error("expected persistK8sNamespaces=true")
	}
	if len(resp.Data.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(resp.Data.Errors))
	}
	if resp.RequestID != "req-ns-1" {
		t.Errorf("expected request_id='req-ns-1', got %q", resp.RequestID)
	}
	if resp.Type != "data" {
		t.Errorf("expected type='data', got %q", resp.Type)
	}
}

func TestPersistNamespaceSourcesResponse_WithErrors(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {
			"persistK8sNamespaces": false,
			"errors": [{"message": "namespace conflict"}, {"message": "quota exceeded"}]
		},
		"request_id": "req-ns-2"
	}`
	var resp persistNamespaceSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.PersistK8sNamespaces {
		t.Error("expected persistK8sNamespaces=false")
	}
	if len(resp.Data.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(resp.Data.Errors))
	}
	if resp.Data.Errors[0].Message != "namespace conflict" {
		t.Errorf("expected first error='namespace conflict', got %q", resp.Data.Errors[0].Message)
	}
	if resp.Data.Errors[1].Message != "quota exceeded" {
		t.Errorf("expected second error='quota exceeded', got %q", resp.Data.Errors[1].Message)
	}
}

func TestPersistNamespaceSourcesResponse_NullErrors(t *testing.T) {
	raw := `{
		"type": "data",
		"data": {"persistK8sNamespaces": true, "errors": null},
		"request_id": "req-ns-3"
	}`
	var resp persistNamespaceSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Data.Errors != nil {
		t.Errorf("expected nil errors, got %v", resp.Data.Errors)
	}
}

func TestPersistNamespaceSourcesResponse_ZeroValues(t *testing.T) {
	raw := `{}`
	var resp persistNamespaceSourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Type != "" || resp.RequestID != "" || resp.Data.PersistK8sNamespaces {
		t.Error("expected zero values for empty JSON")
	}
}

// ---------------------------------------------------------------------------
// NamespaceInput tests
// ---------------------------------------------------------------------------

func TestNamespaceInput_JSON(t *testing.T) {
	raw := `{
		"currentStreamName": "stream-ns",
		"namespace": "production",
		"selected": true
	}`
	var ni NamespaceInput
	if err := json.Unmarshal([]byte(raw), &ni); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if ni.CurrentStreamName != "stream-ns" {
		t.Errorf("expected currentStreamName='stream-ns', got %q", ni.CurrentStreamName)
	}
	if ni.Namespace != "production" {
		t.Errorf("expected namespace='production', got %q", ni.Namespace)
	}
	if !ni.Selected {
		t.Error("expected selected=true")
	}
}

func TestNamespaceInput_RoundTrip(t *testing.T) {
	original := NamespaceInput{
		CurrentStreamName: "stream-b",
		Namespace:         "staging",
		Selected:          false,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded NamespaceInput
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
}

func TestNamespaceInput_ZeroValues(t *testing.T) {
	raw := `{}`
	var ni NamespaceInput
	if err := json.Unmarshal([]byte(raw), &ni); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if ni.CurrentStreamName != "" || ni.Namespace != "" || ni.Selected {
		t.Error("expected zero values")
	}
}

// ---------------------------------------------------------------------------
// Source round-trip test
// ---------------------------------------------------------------------------

func TestSource_RoundTrip(t *testing.T) {
	original := Source{
		Kind:            "Deployment",
		Name:            "web",
		Namespace:       "default",
		DataStreamNames: []string{"stream-1"},
		Selected:        true,
		OtelServiceName: "web-otel",
		Conditions:      []SourceCondition{},
		Containers:      []SourceContainer{},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded Source
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Name != original.Name || decoded.Kind != original.Kind ||
		decoded.Namespace != original.Namespace || decoded.Selected != original.Selected ||
		decoded.OtelServiceName != original.OtelServiceName {
		t.Errorf("round-trip mismatch: %+v != %+v", decoded, original)
	}
	if len(decoded.DataStreamNames) != 1 || decoded.DataStreamNames[0] != "stream-1" {
		t.Errorf("expected dataStreamNames=['stream-1'], got %v", decoded.DataStreamNames)
	}
}
