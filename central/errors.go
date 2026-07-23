package central

import (
	"fmt"

	"github.com/odigos-io/odigos-central-client/operations"
	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/version"
)

// UnsupportedCentralVersionError is returned by NewClient when the connected
// Central server reports a version older than MinCentralVersion. The
// Terraform provider surfaces this as a hard error at Configure time; CLI
// users can read it with errors.As to render a precise message.
type UnsupportedCentralVersionError struct {
	Got version.Version
	Min version.Version
}

func (e *UnsupportedCentralVersionError) Error() string {
	return fmt.Sprintf(
		"odigos central version %s is older than the minimum %s required by this client",
		e.Got, e.Min,
	)
}

// UnsupportedProxyVersionError is returned by CentralClient.Proxy and the
// per-call helpers when a target proxy's advertised odigosVersion is older
// than MinProxyVersion or older than the minimum a particular operation
// needs.
//
// Terraform resources should treat this as a soft, per-cluster failure: emit
// a warning and skip that cluster, leaving compatible clusters to succeed.
type UnsupportedProxyVersionError struct {
	ProxyID  string
	Platform platform.Type
	Got      version.Version
	Min      version.Version
}

func (e *UnsupportedProxyVersionError) Error() string {
	return fmt.Sprintf(
		"proxy %q (%s, %s) is older than the minimum %s required by this client",
		e.ProxyID, e.Platform, e.Got, e.Min,
	)
}

// UnsupportedProxyPlatformError is returned when a proxy advertises a
// compute-platform type the client does not yet support (e.g. a VM cluster
// while VM resources are not yet wired up).
type UnsupportedProxyPlatformError struct {
	ProxyID  string
	Platform string
}

func (e *UnsupportedProxyPlatformError) Error() string {
	return fmt.Sprintf("proxy %q advertises unsupported platform %q", e.ProxyID, e.Platform)
}

// GraphQLErrorLocation is a source location reported for a GraphQL error.
type GraphQLErrorLocation struct {
	Line   int
	Column int
}

// GraphQLErrorDetail is a single entry from a GraphQL response's `errors`
// array. Beyond the human-readable message it retains the machine-readable
// path, locations, and extensions so callers can act on structured failures.
type GraphQLErrorDetail struct {
	Message    string
	Path       []any
	Locations  []GraphQLErrorLocation
	Extensions map[string]any
}

// GraphQLError represents one or more errors reported in a GraphQL response's
// `errors` array (as opposed to a transport-level error).
type GraphQLError struct {
	Op     string
	Errors []GraphQLErrorDetail
}

// Messages returns just the human-readable messages, in order. Handy for
// callers that only need to display or match text.
func (e *GraphQLError) Messages() []string {
	msgs := make([]string, len(e.Errors))
	for i, d := range e.Errors {
		msgs[i] = d.Message
	}
	return msgs
}

func (e *GraphQLError) Error() string {
	switch len(e.Errors) {
	case 0:
		return fmt.Sprintf("graphql %q: unspecified error", e.Op)
	case 1:
		return fmt.Sprintf("graphql %q: %s", e.Op, e.Errors[0].Message)
	default:
		return fmt.Sprintf("graphql %q: %d errors: %v", e.Op, len(e.Errors), e.Messages())
	}
}

// Re-export the operations package error types so callers only need to import
// one package when matching errors.
type (
	UnsupportedOperationPlatformError = operations.UnsupportedPlatformError
	UnsupportedOperationVersionError  = operations.UnsupportedVersionError
)
