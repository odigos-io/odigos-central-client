package operations

import (
	"errors"
	"testing"

	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/version"
)

func TestPickHighestSupported(t *testing.T) {
	op := Operation{
		Name: "GetSource",
		Variants: map[platform.Type]map[version.Version]string{
			platform.K8s: {
				version.MustParse("v1.20"): "v120",
				version.MustParse("v1.22"): "v122",
				version.MustParse("v1.25"): "v125",
			},
		},
	}

	cases := []struct {
		want string
		v    string
	}{
		{"v120", "v1.20"},
		{"v120", "v1.21"},
		{"v122", "v1.22"},
		{"v122", "v1.23"},
		{"v122", "v1.24"},
		{"v125", "v1.25"},
		{"v125", "v1.26"},
		{"v125", "v2.0"},
	}
	for _, c := range cases {
		got, err := op.Pick(platform.K8s, version.MustParse(c.v))
		if err != nil {
			t.Errorf("Pick(K8s, %s) unexpected error: %v", c.v, err)
			continue
		}
		if got != c.want {
			t.Errorf("Pick(K8s, %s) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestPickReturnsUnsupportedVersionError(t *testing.T) {
	op := Operation{
		Name: "GetPeerSources",
		Variants: map[platform.Type]map[version.Version]string{
			platform.K8s: {version.MustParse("v1.21"): "doc"},
		},
	}

	_, err := op.Pick(platform.K8s, version.MustParse("v1.20"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var uve *UnsupportedVersionError
	if !errors.As(err, &uve) {
		t.Fatalf("expected *UnsupportedVersionError, got %T: %v", err, err)
	}
	if uve.Want.String() != "v1.20" || uve.Min.String() != "v1.21" {
		t.Errorf("unexpected error fields: %+v", uve)
	}
}

func TestPickReturnsUnsupportedPlatformError(t *testing.T) {
	op := Operation{
		Name: "RestartWorkloads",
		Variants: map[platform.Type]map[version.Version]string{
			platform.K8s: {version.MustParse("v1.20"): "doc"},
		},
	}

	_, err := op.Pick(platform.Vm, version.MustParse("v0.1"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var upe *UnsupportedPlatformError
	if !errors.As(err, &upe) {
		t.Fatalf("expected *UnsupportedPlatformError, got %T: %v", err, err)
	}
	if upe.Platform != platform.Vm {
		t.Errorf("unexpected platform in error: %q", upe.Platform)
	}
}

func TestPickOnNilOperation(t *testing.T) {
	var op *Operation
	_, err := op.Pick(platform.K8s, version.MustParse("v1.20"))
	if err == nil {
		t.Fatal("expected error from nil Operation")
	}
	var upe *UnsupportedPlatformError
	if !errors.As(err, &upe) {
		t.Fatalf("expected *UnsupportedPlatformError, got %T", err)
	}
}

func TestSupportedPlatformsAndMinVersion(t *testing.T) {
	op := Operation{
		Name: "Multi",
		Variants: map[platform.Type]map[version.Version]string{
			platform.K8s: {
				version.MustParse("v1.20"): "k20",
				version.MustParse("v1.22"): "k22",
			},
			platform.Vm: {
				version.MustParse("v0.1"): "v01",
			},
			"empty": {},
		},
	}

	plats := op.SupportedPlatforms()
	if len(plats) != 2 {
		t.Fatalf("expected 2 supported platforms, got %v", plats)
	}

	min, ok := op.MinVersion(platform.K8s)
	if !ok || min.String() != "v1.20" {
		t.Errorf("MinVersion(K8s) = %v, %v; want v1.20, true", min, ok)
	}
	min, ok = op.MinVersion(platform.Vm)
	if !ok || min.String() != "v0.1" {
		t.Errorf("MinVersion(Vm) = %v, %v; want v0.1, true", min, ok)
	}
	if _, ok := op.MinVersion("missing"); ok {
		t.Errorf("MinVersion for missing platform should report ok=false")
	}
}
