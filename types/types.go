// Package types contains the public Go structs used by the Odigos Central
// client to model GraphQL request inputs and decoded responses.
//
// Design principles:
//
//   - "Lowest common denominator" struct shape across the supported version
//     range (currently K8s v1.20+). Fields that are guaranteed at the floor
//     are non-pointer; fields that may be absent on older proxies, or that
//     were introduced in newer variants, are modelled as pointers / nil-able
//     so a missing JSON field stays nil rather than returning a zero value
//     that could be confused with "explicitly empty".
//
//   - Field tags use `omitempty` for input types so we don't send stale
//     defaults to the server, and `omitempty` on output types where the
//     field may be absent from the response.
//
//   - GraphQL `__typename` fields, when present, are kept as Typename so
//     they can be inspected by callers without breaking unmarshaling.
package types

import "time"

// User is a Central user as embedded in compute-platform and team payloads.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Typename string `json:"__typename,omitempty"`
}

// Team is a Central team as embedded in user and compute-platform payloads.
//
// Description is only present in selections that request it (e.g. signIn's
// user.teams); it stays empty for selections that don't, hence omitempty.
type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Typename    string `json:"__typename,omitempty"`
}

// ComputePlatform describes one cluster (or VM host) connected to Central.
//
// Returned by `computePlatforms` and embedded in the `signIn` user payload.
type ComputePlatform struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OdigosVersion string `json:"odigosVersion"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	ConnectedAt   int64  `json:"connectedAt"`
	LastSeenAt    int64  `json:"lastSeenAt"`
	Users         []User `json:"users"`
	Teams         []Team `json:"teams"`
	Typename      string `json:"__typename,omitempty"`
}

// SystemConfig is returned by the `systemConfig` query and is the canonical
// source for the Central server version that the client uses for the
// version handshake.
type SystemConfig struct {
	IsInitialized bool   `json:"isInitialized"`
	Version       string `json:"version"`
}

// SignInUser is the user object returned in a successful signIn payload.
type SignInUser struct {
	ID                  string            `json:"id"`
	Email               string            `json:"email"`
	Username            string            `json:"username"`
	Role                string            `json:"role"`
	NeedsPasswordChange bool              `json:"needsPasswordChange"`
	Teams               []Team            `json:"teams"`
	ComputePlatforms    []ComputePlatform `json:"computePlatforms"`
	Typename            string            `json:"__typename,omitempty"`
}

// SignInResult is the data returned by the signIn mutation.
type SignInResult struct {
	IDToken     string     `json:"idToken"`
	AccessToken string     `json:"accessToken"`
	Status      string     `json:"status"`
	User        SignInUser `json:"user"`
	Typename    string     `json:"__typename,omitempty"`
}

// SourceContainer describes a single container of an instrumented source.
type SourceContainer struct {
	ContainerName          string `json:"containerName"`
	InstrumentationMessage string `json:"instrumentationMessage"`
	Instrumented           bool   `json:"instrumented"`
	Language               string `json:"language"`
	OtelDistroName         string `json:"otelDistroName"`
	Overridden             bool   `json:"overriden"`
	RuntimeVersion         string `json:"runtimeVersion"`
}

// SourceCondition is one entry in the conditions[] field on a source.
//
// LastTransitionTime is omitted when missing rather than defaulting to the
// zero time, which would silently look like 1970.
type SourceCondition struct {
	LastTransitionTime *time.Time `json:"lastTransitionTime,omitempty"`
	Message            string     `json:"message"`
	Reason             string     `json:"reason"`
	Status             string     `json:"status"`
	Type               string     `json:"type"`
}

// Source is the canonical shape of a single cluster source.
//
// Pointer fields are version-conditional: at K8s v1.20+, manifestYAML and
// instrumentationConfigYAML are populated by GetSource; both will be nil for
// summary lookups (GetSources) or older variants that did not request them.
type Source struct {
	Conditions      []SourceCondition `json:"conditions,omitempty"`
	Containers      []SourceContainer `json:"containers,omitempty"`
	DataStreamNames []string          `json:"dataStreamNames"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	OtelServiceName string            `json:"otelServiceName"`
	Selected        bool              `json:"selected"`

	// Detail-only fields. Populated by GetSource (single), nil from GetSources.
	ManifestYAML              *string `json:"manifestYAML,omitempty"`
	InstrumentationConfigYAML *string `json:"instrumentationConfigYAML,omitempty"`
}

// SourceID identifies a source for read/write operations.
//
// Note: the GraphQL input type is named `K8sSourceId` (a Kubernetes-specific
// identifier shape). We expose it as `SourceID` because for the K8s-only
// scope of the current client there is one canonical identifier.
type SourceID struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SourceInput is the input shape for persistK8sSources.
type SourceInput struct {
	CurrentStreamName string `json:"currentStreamName"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Selected          bool   `json:"selected"`
}

// K8sActualNamespace is the lite namespace shape returned by
// `computePlatform.k8sActualNamespaces`.
type K8sActualNamespace struct {
	Name            string   `json:"name"`
	Selected        bool     `json:"selected"`
	DataStreamNames []string `json:"dataStreamNames"`
}

// NamespaceSource is one source listed under a namespace's `sources` field.
type NamespaceSource struct {
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	Name              string   `json:"name"`
	DataStreamNames   []string `json:"dataStreamNames"`
	Selected          bool     `json:"selected"`
	NumberOfInstances int      `json:"numberOfInstances"`
}

// K8sActualNamespaceDetail is the detail shape returned by
// `computePlatform.k8sActualNamespace(name:)`.
type K8sActualNamespaceDetail struct {
	Name            string            `json:"name"`
	Selected        bool              `json:"selected"`
	DataStreamNames []string          `json:"dataStreamNames"`
	Sources         []NamespaceSource `json:"sources,omitempty"`
}

// NamespaceInput is the input shape for persistK8sNamespaces.
type NamespaceInput struct {
	CurrentStreamName string `json:"currentStreamName"`
	Namespace         string `json:"namespace"`
	Selected          bool   `json:"selected"`
}
