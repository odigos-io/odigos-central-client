// Package version provides parsing and comparison of Odigos semantic-ish version
// strings as they appear in Central GraphQL payloads.
//
// Odigos versions surface in two places:
//   - systemConfig.version (the Central server version)
//   - computePlatform.odigosVersion (the per-cluster proxy version)
//
// The strings can take any of these forms: "v1.20", "v1.20.0", "v1.22.0-rc0",
// "1.20", or "1.20.0". This package treats only the (major, minor) pair as
// significant, mirroring the central-ui trimVersion helper that the UI uses
// when picking GraphQL query variants.
package version

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version captures the (major, minor) pair of an Odigos release.
//
// Patch and pre-release tags are intentionally discarded because the GraphQL
// schema versioning model used by Central treats v1.20.0, v1.20.1 and
// v1.20.0-rc0 as schema-equivalent.
type Version struct {
	Major int
	Minor int
}

// versionPattern accepts an optional leading "v", a major.minor, and ignores
// anything trailing (".patch", "-rc0", etc.).
var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// Parse converts an Odigos version string into a Version.
// It is forgiving about leading "v", patch components and pre-release suffixes.
//
// Returns an error if the string cannot be interpreted as major.minor.
func Parse(s string) (Version, error) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("version: cannot parse %q as major.minor", s)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return Version{}, fmt.Errorf("version: invalid major in %q: %w", s, err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return Version{}, fmt.Errorf("version: invalid minor in %q: %w", s, err)
	}
	return Version{Major: major, Minor: minor}, nil
}

// MustParse is the panicking variant of Parse, intended for package-level
// constants where the input is known to be well-formed.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// Compare returns -1, 0, or 1 if v is less than, equal to, or greater than o.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		if v.Major < o.Major {
			return -1
		}
		return 1
	}
	if v.Minor != o.Minor {
		if v.Minor < o.Minor {
			return -1
		}
		return 1
	}
	return 0
}

// GTE reports whether v is greater than or equal to o.
func (v Version) GTE(o Version) bool { return v.Compare(o) >= 0 }

// LT reports whether v is strictly less than o.
func (v Version) LT(o Version) bool { return v.Compare(o) < 0 }

// String renders the canonical "vMAJOR.MINOR" form.
func (v Version) String() string { return fmt.Sprintf("v%d.%d", v.Major, v.Minor) }
