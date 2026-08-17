package operations

import (
	"strings"
	"testing"

	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/version"
)

// minK8s is the K8s version floor enforced by the generator. The
// compatibility matrix verifies the generated operations conform to it.
var minK8s = version.MustParse("v1.20")

// TestCompatibilityMatrix_RemoteOpsHaveVariants asserts that every generated
// remote operation has at least one platform with at least one variant. An
// operation with all empty variant maps would be unreachable.
func TestCompatibilityMatrix_RemoteOpsHaveVariants(t *testing.T) {
	all := append([]*Operation{}, AllRemoteQueries...)
	all = append(all, AllRemoteMutations...)
	if len(all) == 0 {
		t.Fatal("no remote operations registered")
	}
	for _, op := range all {
		if len(op.Variants) == 0 {
			t.Errorf("operation %q has no variants", op.Name)
			continue
		}
		hasAny := false
		for _, perPlat := range op.Variants {
			if len(perPlat) > 0 {
				hasAny = true
				break
			}
		}
		if !hasAny {
			t.Errorf("operation %q has only empty variant maps", op.Name)
		}
	}
}

// TestCompatibilityMatrix_K8sVariantsRespectFloor asserts that no K8s variant
// declares a minimum below the configured floor (v1.20). A variant below the
// floor should have been folded up by filterVariants in the generator.
func TestCompatibilityMatrix_K8sVariantsRespectFloor(t *testing.T) {
	all := append([]*Operation{}, AllRemoteQueries...)
	all = append(all, AllRemoteMutations...)
	for _, op := range all {
		k8s, ok := op.Variants[platform.K8s]
		if !ok {
			continue
		}
		for v := range k8s {
			if v.LT(minK8s) {
				t.Errorf("operation %q has K8s variant %s, below floor %s", op.Name, v, minK8s)
			}
		}
	}
}

// TestCompatibilityMatrix_PickAtMinSucceeds asserts that for every remote
// operation that supports K8s, Pick(K8s, op-min-version) returns a non-empty
// document. This guarantees the version routing is well-formed end-to-end:
// any cluster running at-or-above an operation's minimum can use it.
//
// Operations whose minimum is above the v1.20 client floor are still
// expected to surface a clean *UnsupportedVersionError to v1.20 callers; the
// per-resource Terraform code treats that as a soft, per-feature skip.
func TestCompatibilityMatrix_PickAtMinSucceeds(t *testing.T) {
	all := append([]*Operation{}, AllRemoteQueries...)
	all = append(all, AllRemoteMutations...)
	for _, op := range all {
		if _, ok := op.Variants[platform.K8s]; !ok {
			continue
		}
		opMin, ok := op.MinVersion(platform.K8s)
		if !ok {
			t.Errorf("operation %q: K8s variants present but MinVersion returned !ok", op.Name)
			continue
		}
		doc, err := op.Pick(platform.K8s, opMin)
		if err != nil {
			t.Errorf("operation %q: Pick(K8s, %s) failed: %v", op.Name, opMin, err)
			continue
		}
		if doc == "" {
			t.Errorf("operation %q: Pick(K8s, %s) returned empty doc", op.Name, opMin)
		}
	}
}

// TestCompatibilityMatrix_PreV120OperationsBaseline asserts that operations
// that already existed before v1.20 (i.e. were rebased up by the generator)
// still resolve at v1.20. This protects against generator regressions where
// the rebase folding might lose its baseline.
func TestCompatibilityMatrix_PreV120OperationsBaseline(t *testing.T) {
	// Every operation the provider currently relies on lives in the v1.11
	// rebased family in the upstream UI; expect Pick(K8s, v1.20) to succeed
	// for these specific, known-stable operations.
	stable := []*Operation{
		&GET_SOURCES_WITH_STATUS, &GET_SOURCE, &GET_NAMESPACES_WITH_SOURCES,
		&PERSIST_SOURCES, &PERSIST_NAMESPACES,
	}
	for _, op := range stable {
		doc, err := op.Pick(platform.K8s, minK8s)
		if err != nil {
			t.Errorf("stable operation %q: Pick(K8s, %s) failed: %v", op.Name, minK8s, err)
			continue
		}
		if doc == "" {
			t.Errorf("stable operation %q: Pick(K8s, %s) returned empty doc", op.Name, minK8s)
		}
	}
}

// TestCompatibilityMatrix_NoUnresolvedInterpolations guards against a generator
// regression where a TypeScript template interpolation (${FRAGMENT} or a
// builder call) is emitted verbatim instead of being inlined. Such a document
// is invalid GraphQL and would fail at runtime, so every registered variant
// must be free of `${` sequences.
func TestCompatibilityMatrix_NoUnresolvedInterpolations(t *testing.T) {
	all := append([]*Operation{}, AllRemoteQueries...)
	all = append(all, AllRemoteMutations...)
	for _, op := range all {
		for pt, variants := range op.Variants {
			for v, doc := range variants {
				if strings.Contains(doc, "${") {
					t.Errorf("operation %q variant (%s, %s) has an unresolved interpolation ${...}", op.Name, pt, v)
				}
			}
		}
	}
}

// TestCompatibilityMatrix_OperationNamesAreUnique guards against a generator
// regression that would silently overwrite an operation declared in two
// places.
func TestCompatibilityMatrix_OperationNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	all := append([]*Operation{}, AllRemoteQueries...)
	all = append(all, AllRemoteMutations...)
	for _, op := range all {
		if seen[op.Name] {
			t.Errorf("duplicate operation name %q", op.Name)
		}
		seen[op.Name] = true
	}
}
