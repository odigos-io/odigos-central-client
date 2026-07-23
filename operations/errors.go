package operations

import (
	"fmt"

	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/version"
)

// UnsupportedPlatformError is returned by Operation.Pick when the operation
// has no variants for the requested platform type. Callers can use errors.As
// to surface a clear "this feature isn't supported for VM clusters" message.
type UnsupportedPlatformError struct {
	Op       string
	Platform platform.Type
}

func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("operation %q is not supported for platform %q", e.Op, e.Platform)
}

// UnsupportedVersionError is returned by Operation.Pick when the operation
// has variants for the requested platform but the proxy's advertised version
// is older than the minimum that introduced any of them.
type UnsupportedVersionError struct {
	Op       string
	Platform platform.Type
	Want     version.Version
	Min      version.Version
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf(
		"operation %q is not supported on %s %s; minimum supported version is %s",
		e.Op, e.Platform, e.Want, e.Min,
	)
}
