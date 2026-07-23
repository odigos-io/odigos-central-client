package main

import (
	"strings"
	"testing"

	"github.com/odigos-io/odigos-central-client/version"
)

const sampleCentralFile = `import { gql } from '@apollo/client';

// A leading comment.
export const GET_FOO = gql` + "`" + `
  query GetFoo {
    foo {
      id
    }
  }
` + "`" + `;

/* Block
   comment */
export const SET_BAR = gql` + "`" + `
  mutation SetBar($id: ID!) {
    setBar(id: $id) { ok }
  }
` + "`" + `;
`

const sampleRemoteFile = `import type { VersionedRemoteFetch } from '@/types';
import { PlatformType } from '@odigos/ui-kit/types';

export const GET_THINGS: VersionedRemoteFetch = {
  [PlatformType.K8s]: {
    'v1.22': ` + "`" + `query GetThings { things { a } }` + "`" + `,
    'v1.11': ` + "`" + `query GetThings { things { legacy } }` + "`" + `,
  },
  [PlatformType.Vm]: {
    'v0.1': ` + "`" + `query GetThings { things { vm } }` + "`" + `,
  },
};

export const PERSIST_THING: VersionedRemoteFetch = {
  [PlatformType.K8s]: {
    'v1.20': ` + "`" + `mutation PersistThing { persistThing }` + "`" + `,
  },
};
`

func newTestResolver() *resolver {
	return &resolver{fragments: map[string]string{}, builders: map[string]builder{}}
}

func TestExtractDeclarations_Central(t *testing.T) {
	src := stripComments(sampleCentralFile)
	gqls, remotes := extractDeclarations(src, newTestResolver())
	if len(remotes) != 0 {
		t.Errorf("expected 0 remote ops, got %d", len(remotes))
	}
	if len(gqls) != 2 {
		t.Fatalf("expected 2 gql ops, got %d", len(gqls))
	}
	if gqls[0].Name != "GET_FOO" || !strings.Contains(gqls[0].Doc, "query GetFoo") {
		t.Errorf("unexpected GET_FOO: %+v", gqls[0])
	}
	if gqls[1].Name != "SET_BAR" || !strings.Contains(gqls[1].Doc, "mutation SetBar") {
		t.Errorf("unexpected SET_BAR: %+v", gqls[1])
	}
}

func TestExtractDeclarations_Remote(t *testing.T) {
	src := stripComments(sampleRemoteFile)
	gqls, remotes := extractDeclarations(src, newTestResolver())
	if len(gqls) != 0 {
		t.Errorf("expected 0 gql ops, got %d", len(gqls))
	}
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remote ops, got %d", len(remotes))
	}

	get := remotes[0]
	if get.Name != "GET_THINGS" {
		t.Fatalf("first op name = %q", get.Name)
	}
	if len(get.Variants["K8s"]) != 2 {
		t.Errorf("expected 2 K8s variants, got %d", len(get.Variants["K8s"]))
	}
	if !strings.Contains(get.Variants["K8s"]["v1.22"], "things { a }") {
		t.Errorf("v1.22 doc unexpected: %q", get.Variants["K8s"]["v1.22"])
	}
	if !strings.Contains(get.Variants["K8s"]["v1.11"], "things { legacy }") {
		t.Errorf("v1.11 doc unexpected: %q", get.Variants["K8s"]["v1.11"])
	}
	if !strings.Contains(get.Variants["Vm"]["v0.1"], "things { vm }") {
		t.Errorf("Vm v0.1 doc unexpected: %q", get.Variants["Vm"]["v0.1"])
	}
}

func TestFilterVariants_RebasesBelowMin(t *testing.T) {
	op := remoteOp{
		Name: "GET_THINGS",
		Variants: map[string]map[string]string{
			"K8s": {
				"v1.11": "legacy",
				"v1.22": "modern",
			},
			"Vm": {
				"v0.1": "vm",
			},
		},
	}
	mins := map[string]version.Version{
		"K8s": version.MustParse("v1.20"),
		"Vm":  version.MustParse("v0.1"),
	}
	out := filterVariants([]remoteOp{op}, mins)
	if len(out) != 1 {
		t.Fatalf("expected 1 op, got %d", len(out))
	}
	got := out[0]

	// v1.11 dropped, v1.22 kept, baseline (v1.11) re-keyed to v1.20.
	k8s := got.Variants["K8s"]
	if len(k8s) != 2 {
		t.Errorf("expected 2 K8s variants after rebase, got %d (%v)", len(k8s), k8s)
	}
	if k8s["v1.20"] != "legacy" {
		t.Errorf("expected baseline 'legacy' at v1.20, got %q", k8s["v1.20"])
	}
	if k8s["v1.22"] != "modern" {
		t.Errorf("expected 'modern' at v1.22, got %q", k8s["v1.22"])
	}
	if _, exists := k8s["v1.11"]; exists {
		t.Errorf("v1.11 should have been removed")
	}

	if got.Variants["Vm"]["v0.1"] != "vm" {
		t.Errorf("Vm variant lost: %v", got.Variants["Vm"])
	}
}

func TestFilterVariants_DropsOpWithNoVariants(t *testing.T) {
	op := remoteOp{
		Name: "ONLY_OBSOLETE",
		Variants: map[string]map[string]string{
			"Bogus": {
				"v9.9": "ignored",
			},
		},
	}
	out := filterVariants([]remoteOp{op}, map[string]version.Version{
		"K8s": version.MustParse("v1.20"),
	})
	// Bogus has no min in the map so it's kept.
	if len(out) != 1 {
		t.Fatalf("expected 1 op (no-min platforms are kept), got %d", len(out))
	}
}

// sampleFragmentFile exercises fragment interpolation, nested fragments, and
// an arrow-function builder used as a variant value (with a defaulted boolean
// parameter driving a ternary).
const sampleFragmentFile = "" +
	"const SCOPE = `sources { namespace kind name }`;\n" +
	"const RULE_FIELDS = `ruleId sourceScopes { ${SCOPE} } notes`;\n" +
	"const EXTRA = `rollout { status }`;\n" +
	"const buildQuery = (fields: string, includeExtra = false) => `\n" +
	"  query GetRules {\n" +
	"    rules { ${fields} }\n" +
	"    ${includeExtra ? EXTRA : ''}\n" +
	"  }`;\n" +
	"export const GET_RULES: VersionedRemoteFetch = {\n" +
	"  [PlatformType.K8s]: {\n" +
	"    'v1.24': buildQuery(RULE_FIELDS),\n" +
	"    'v1.30': buildQuery(RULE_FIELDS, true),\n" +
	"  },\n" +
	"};\n"

func TestFragmentAndBuilderResolution(t *testing.T) {
	src := stripComments(sampleFragmentFile)
	r := &resolver{fragments: extractFragments(src), builders: extractBuilders(src)}

	// Fragments (including the arrow builder's referenced fragments) are found,
	// but the builder itself must not be collected as a fragment.
	if _, ok := r.fragments["SCOPE"]; !ok {
		t.Fatalf("SCOPE fragment not collected: %v", r.fragments)
	}
	if _, ok := r.fragments["buildQuery"]; ok {
		t.Errorf("arrow builder was wrongly collected as a fragment")
	}
	if _, ok := r.builders["buildQuery"]; !ok {
		t.Fatalf("buildQuery builder not collected: %v", r.builders)
	}

	_, remotes := extractDeclarations(src, r)
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote op, got %d", len(remotes))
	}
	variants := remotes[0].Variants["K8s"]

	// Nested fragment resolution: ${RULE_FIELDS} -> ... ${SCOPE} -> sources{...}.
	v124 := variants["v1.24"]
	for _, want := range []string{"ruleId", "sources { namespace kind name }", "notes"} {
		if !strings.Contains(v124, want) {
			t.Errorf("v1.24 missing %q; got:\n%s", want, v124)
		}
	}
	// Defaulted boolean parameter => false => ternary yields empty; no EXTRA.
	if strings.Contains(v124, "rollout") {
		t.Errorf("v1.24 should not include EXTRA (default false); got:\n%s", v124)
	}
	// Explicit true argument => ternary yields EXTRA fragment.
	if !strings.Contains(variants["v1.30"], "rollout { status }") {
		t.Errorf("v1.30 should include EXTRA; got:\n%s", variants["v1.30"])
	}
	// No interpolation must survive resolution.
	for ver, doc := range variants {
		if strings.Contains(doc, "${") {
			t.Errorf("variant %s still has unresolved interpolation:\n%s", ver, doc)
		}
	}
}

func TestStripComments(t *testing.T) {
	in := "// line\n/* block */code\nstr=`/* not a comment */`;\n"
	out := stripComments(in)
	if strings.Contains(out, "// line") {
		t.Errorf("line comment not stripped: %q", out)
	}
	if strings.Contains(out, "/* block */") {
		t.Errorf("block comment not stripped: %q", out)
	}
	if !strings.Contains(out, "/* not a comment */") {
		t.Errorf("template-literal contents wrongly stripped: %q", out)
	}
}
