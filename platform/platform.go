// Package platform models the compute-platform types that Central exposes
// (Kubernetes clusters, VM hosts, ...). The same enum is used by Central's
// GraphQL schema (PlatformType) and by the central-ui VersionedRemoteFetch
// map; we mirror it here so the Go client can route operations correctly.
package platform

import (
	"fmt"
	"strings"
)

// Type identifies the kind of compute platform a proxy represents.
//
// New values must be added in lock-step with Central's PlatformType enum.
// Unknown values returned by the server are surfaced as Type("") plus an
// error from Parse, rather than silently being treated as Kubernetes.
type Type string

const (
	// K8s is a Kubernetes cluster proxy. Every variant in the central-ui
	// VersionedRemoteFetch maps that uses [PlatformType.K8s] is keyed on
	// this value.
	K8s Type = "k8s"

	// Vm is a virtual-machine / non-Kubernetes proxy. Defined for forward
	// compatibility; the Go client and Terraform provider currently do not
	// drive any VM-specific resources.
	Vm Type = "vm"
)

// Parse converts the raw `computePlatform.type` string from the GraphQL API
// into a Type. The comparison is case-insensitive to be tolerant of casing
// differences across Central versions, but the canonical lower-case form is
// what's stored in the returned Type.
func Parse(s string) (Type, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "k8s", "kubernetes":
		return K8s, nil
	case "vm":
		return Vm, nil
	}
	return Type(""), fmt.Errorf("platform: unknown compute platform type %q", s)
}

// String returns the canonical string representation of the platform type.
func (t Type) String() string { return string(t) }
