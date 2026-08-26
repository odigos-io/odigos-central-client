// gen-graphql parses Central UI GraphQL operation files and emits matching
// Go declarations for the odigos-central-client `operations` package.
//
// Supported input shapes (drawn from /odigos-enterprise/central-ui/src/graphql):
//
//	export const FOO = gql`
//	  query Foo { ... }
//	`;
//
//	export const BAR: VersionedRemoteFetch = {
//	  [PlatformType.K8s]: {
//	    'v1.20': `
//	      query Bar { ... }
//	    `,
//	    'v1.22': `...`,
//	  },
//	  [PlatformType.Vm]: {
//	    'v0.1': `...`,
//	  },
//	};
//
// The resulting Go file declares for each `gql` constant a string variable,
// and for each VersionedRemoteFetch constant an `operations.Operation` value.
//
// Variants whose minimum version is below the configured floor (default
// k8s/v1.20) are dropped; operations that end up with no variants for any
// platform are skipped.
//
// Usage:
//
//	go run ./tools/gen-graphql \
//	  -source /path/to/odigos-enterprise/central-ui/src/graphql \
//	  -out    ./operations \
//	  -min-k8s v1.20
//
// The tool is conservative: any constant whose RHS does not match one of the
// two recognised shapes is skipped with a warning, never silently miscompiled.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/odigos-io/odigos-central-client/version"
)

// CLI flags.
type flags struct {
	source string
	out    string
	minK8s string
	minVm  string
}

// gqlOp captures a top-level exported gql tagged-template constant of the form
// export const NAME = gql...
type gqlOp struct {
	Name string
	Doc  string
}

// remoteOp captures an `export const NAME: VersionedRemoteFetch = {...}` constant.
type remoteOp struct {
	Name     string
	Variants map[string]map[string]string // platform -> version (e.g. "v1.20") -> document
}

func main() {
	f := parseFlags()
	if err := run(f); err != nil {
		log.Fatalf("gen-graphql: %v", err)
	}
}

func parseFlags() flags {
	var f flags
	flag.StringVar(&f.source, "source", "", "Path to central-ui/src/graphql (or odigos-enterprise root; auto-detected)")
	flag.StringVar(&f.out, "out", "operations", "Output directory for generated Go files")
	flag.StringVar(&f.minK8s, "min-k8s", "v1.20", "Minimum K8s variant to retain")
	flag.StringVar(&f.minVm, "min-vm", "v0.1", "Minimum VM variant to retain")
	flag.Parse()
	if f.source == "" {
		fmt.Fprintln(os.Stderr, "gen-graphql: -source is required")
		os.Exit(2)
	}
	return f
}

func run(f flags) error {
	root, err := resolveGraphqlRoot(f.source)
	if err != nil {
		return err
	}

	minK8s, err := version.Parse(f.minK8s)
	if err != nil {
		return fmt.Errorf("invalid -min-k8s: %w", err)
	}
	minVm, err := version.Parse(f.minVm)
	if err != nil {
		return fmt.Errorf("invalid -min-vm: %w", err)
	}
	mins := map[string]version.Version{"K8s": minK8s, "Vm": minVm}

	queriesDir := filepath.Join(root, "queries")
	mutationsDir := filepath.Join(root, "mutations")

	// Fragments (plain template-literal string constants interpolated into
	// operations via ${NAME}) and arrow-function query builders (used as
	// variant values, e.g. `'v1.26': getSamplingRulesQuery(FIELDS)`) may live
	// anywhere under the graphql root, so collect them across the whole tree
	// before resolving any document.
	r, err := newResolver(root)
	if err != nil {
		return fmt.Errorf("collect fragments/builders: %w", err)
	}

	queryGql, queryRemote, err := parseTree(queriesDir, r)
	if err != nil {
		return fmt.Errorf("parse queries: %w", err)
	}
	mutationGql, mutationRemote, err := parseTree(mutationsDir, r)
	if err != nil {
		return fmt.Errorf("parse mutations: %w", err)
	}

	// Central scope: gql constants from non-remote files.
	centralOps := append([]gqlOp{}, queryGql...)
	centralOps = append(centralOps, mutationGql...)
	dedupSortGql(&centralOps)

	queryRemote = filterVariants(queryRemote, mins)
	mutationRemote = filterVariants(mutationRemote, mins)
	dedupSortRemote(&queryRemote)
	dedupSortRemote(&mutationRemote)

	// Documents are fully resolved during parsing; do a final pass to warn
	// about any interpolation we could not resolve (which would emit invalid
	// GraphQL) so a generator regression is loud rather than silent.
	warnUnresolvedOps(centralOps, queryRemote, mutationRemote)

	if err := os.MkdirAll(f.out, 0o755); err != nil {
		return err
	}

	if err := writeGoFile(filepath.Join(f.out, "central_ops.go"), centralTemplate, centralOps); err != nil {
		return err
	}
	if err := writeGoFile(filepath.Join(f.out, "remote_queries.go"), remoteTemplate("remote_queries"), queryRemote); err != nil {
		return err
	}
	if err := writeGoFile(filepath.Join(f.out, "remote_mutations.go"), remoteTemplate("remote_mutations"), mutationRemote); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "gen-graphql: wrote %d central, %d remote queries, %d remote mutations\n",
		len(centralOps), len(queryRemote), len(mutationRemote))
	return nil
}

// resolveGraphqlRoot accepts either the graphql root directly, the central-ui
// dir, or an odigos-enterprise repo root; picks the first existing layout.
func resolveGraphqlRoot(p string) (string, error) {
	candidates := []string{
		p,
		filepath.Join(p, "central-ui", "src", "graphql"),
		filepath.Join(p, "src", "graphql"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(c, "queries")); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("could not locate central-ui/src/graphql under %q", p)
}

// parseTree walks a queries/ or mutations/ directory and extracts both gql
// constants (one bucket) and VersionedRemoteFetch constants (another bucket).
// Documents are resolved through r (fragment + builder inlining) as they are
// parsed.
func parseTree(dir string, r *resolver) ([]gqlOp, []remoteOp, error) {
	var gqls []gqlOp
	var remotes []remoteOp

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".ts") {
			return nil
		}
		if d.Name() == "index.ts" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := stripComments(string(raw))

		fileGqls, fileRemotes := extractDeclarations(src, r)
		gqls = append(gqls, fileGqls...)
		remotes = append(remotes, fileRemotes...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return gqls, remotes, nil
}

var (
	exportConstRe = regexp.MustCompile(`(?m)^\s*export\s+const\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*([A-Za-z_][A-Za-z0-9_]*))?\s*=\s*`)
	platformKeyRe = regexp.MustCompile(`\[\s*PlatformType\.([A-Za-z0-9_]+)\s*\]\s*:\s*\{`)
	variantKeyRe  = regexp.MustCompile(`'([vV]\d+\.\d+)'\s*:\s*`)

	// fragmentConstRe matches a plain template-literal string constant, e.g.
	// `const WORKLOAD_FIELDS = ` + "`...`" + `. It deliberately requires a
	// backtick immediately after `=` so it does not match `gql`...`` operations
	// (which start with `gql`) or `VersionedRemoteFetch` object literals.
	fragmentConstRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*` + "`")
	// builderConstRe matches an arrow-function query builder, e.g.
	// `const getSamplingRulesQuery = (noisyFields: string) => ` + "`...`" + `.
	builderConstRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\(([^)]*)\)\s*=>\s*` + "`")
)

// collectFragments walks the entire graphql tree and returns a map of
// template-literal string constants (fragments) keyed by name. These are
// `const NAME = ` + "`...`" + ` declarations that are neither `gql` operations
// nor VersionedRemoteFetch maps; operation documents interpolate them via
// ${NAME}.
func collectFragments(root string) (map[string]string, error) {
	frags := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".ts") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := stripComments(string(raw))
		for name, doc := range extractFragments(src) {
			frags[name] = doc
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return frags, nil
}

// extractFragments pulls plain template-literal string constants out of a
// single file's (comment-stripped) source.
func extractFragments(src string) map[string]string {
	out := map[string]string{}
	for _, m := range fragmentConstRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		// The opening backtick is the final char the regex matched, so the
		// literal body begins at m[1].
		start := m[1]
		end := findTemplateEnd(src, start)
		if end < 0 {
			continue
		}
		out[name] = strings.TrimSpace(src[start:end])
	}
	return out
}

// builderParam is one parameter of an arrow-function query builder.
type builderParam struct {
	Name       string
	HasDefault bool
	// Default is the raw default expression as written in the source, e.g.
	// "false" (boolean) or "''" (empty string). It is interpreted the same way
	// as a call argument when the parameter is omitted at the call site.
	Default string
}

// builder captures an arrow-function query builder, e.g.
//
//	const getSamplingRulesQuery = (noisyFields: string) => `query ...`;
//
// Its body interpolates ${param} placeholders and, optionally, simple
// ternaries of the form ${cond ? FRAGMENT : empty} whose condition is a
// boolean parameter.
type builder struct {
	Name   string
	Params []builderParam
	Body   string
}

// resolver inlines fragment and arrow-function-builder interpolations so the
// generated Go contains complete, valid GraphQL documents rather than raw
// TypeScript template expressions.
type resolver struct {
	fragments map[string]string
	builders  map[string]builder
}

// callValue is a bound argument to a builder call: either GraphQL text (from a
// fragment) or a boolean literal (for ternary conditions).
type callValue struct {
	text   string
	isBool bool
	b      bool
}

// newResolver walks the whole graphql tree collecting fragments and builders.
func newResolver(root string) (*resolver, error) {
	frags, err := collectFragments(root)
	if err != nil {
		return nil, err
	}
	builders, err := collectBuilders(root)
	if err != nil {
		return nil, err
	}
	return &resolver{fragments: frags, builders: builders}, nil
}

// collectBuilders walks the tree and returns arrow-function query builders of
// the form `const NAME = (params) => ` + "`...`" + `.
func collectBuilders(root string) (map[string]builder, error) {
	out := map[string]builder{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".ts") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := stripComments(string(raw))
		for name, b := range extractBuilders(src) {
			out[name] = b
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// extractBuilders pulls arrow-function template builders out of a single
// file's (comment-stripped) source.
func extractBuilders(src string) map[string]builder {
	out := map[string]builder{}
	for _, m := range builderConstRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		params := parseParams(src[m[4]:m[5]])
		start := m[1] // just past the opening backtick
		end := findTemplateEnd(src, start)
		if end < 0 {
			continue
		}
		out[name] = builder{Name: name, Params: params, Body: strings.TrimSpace(src[start:end])}
	}
	return out
}

// parseParams parses a comma-separated arrow-function parameter list, dropping
// TypeScript type annotations and recording boolean defaults.
func parseParams(list string) []builderParam {
	list = strings.TrimSpace(list)
	if list == "" {
		return nil
	}
	var out []builderParam
	for _, raw := range strings.Split(list, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p := builderParam{}
		// Split off a default value if present.
		if eq := strings.IndexByte(raw, '='); eq >= 0 {
			p.HasDefault = true
			p.Default = strings.TrimSpace(raw[eq+1:])
			raw = strings.TrimSpace(raw[:eq])
		}
		// Drop a `: type` annotation, keeping only the identifier.
		if colon := strings.IndexByte(raw, ':'); colon >= 0 {
			raw = strings.TrimSpace(raw[:colon])
		}
		p.Name = raw
		if p.Name != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolve inlines every ${...} interpolation in doc using only fragments (no
// bound parameters). Used for plain documents and fragment bodies.
func (r *resolver) resolve(doc string) string {
	return r.resolveEnv(doc, nil)
}

// resolveEnv inlines ${...} interpolations, first consulting env (bound
// builder parameters) then fragments. It repeats to a fixpoint so an inlined
// fragment may itself contain further interpolations. Unresolvable
// interpolations are left in place so callers can warn about them.
func (r *resolver) resolveEnv(doc string, env map[string]callValue) string {
	for i := 0; i < 50; i++ {
		next, changed := r.expandOnce(doc, env)
		doc = next
		if !changed || !strings.Contains(doc, "${") {
			break
		}
	}
	return doc
}

// skipQuoted returns the index just past a single- or double-quoted string
// literal whose opening quote is at s[i]. Backslash escapes are honored. A
// backtick template literal is not handled here (see findTemplateEnd), because
// it may itself embed ${...} interpolations. Returns len(s) if unterminated.
func skipQuoted(s string, i int) int {
	q := s[i]
	i++
	for i < len(s) {
		switch s[i] {
		case '\\':
			i += 2
			continue
		case q:
			return i + 1
		}
		i++
	}
	return len(s)
}

// findTemplateEnd returns the index of the backtick that closes a template
// literal whose body begins at s[i] (i.e. i is just past the opening backtick).
// It skips over backslash escapes and over ${...} interpolations (which may
// contain nested braces, quoted strings, and further template literals).
// Returns -1 if the literal is unterminated.
func findTemplateEnd(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case '\\':
			i += 2
			continue
		case '`':
			return i
		case '$':
			if i+1 < len(s) && s[i+1] == '{' {
				j := matchInterp(s, i+1)
				if j < 0 {
					return -1
				}
				i = j + 1
				continue
			}
		}
		i++
	}
	return -1
}

// matchInterp returns the index of the '}' that closes an interpolation whose
// opening '{' is at s[open] (the '{' of a "${"). Brace depth is balanced while
// skipping quoted strings and nested template literals so a '}' inside a string
// or template is never mistaken for the terminator. Returns -1 if unbalanced.
func matchInterp(s string, open int) int {
	depth := 0
	i := open
	for i < len(s) {
		switch s[i] {
		case '\\':
			i += 2
			continue
		case '\'', '"':
			i = skipQuoted(s, i)
			continue
		case '`':
			end := findTemplateEnd(s, i+1)
			if end < 0 {
				return -1
			}
			i = end + 1
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

// indexTopLevel returns the index of the first occurrence of b in s that is not
// inside a quoted string or template literal, or -1. Used to split ternaries so
// a '?' or ':' appearing inside a string branch is not treated as a delimiter.
func indexTopLevel(s string, b byte) int {
	i := 0
	for i < len(s) {
		switch c := s[i]; c {
		case '\\':
			i += 2
			continue
		case '\'', '"':
			i = skipQuoted(s, i)
			continue
		case '`':
			end := findTemplateEnd(s, i+1)
			if end < 0 {
				return -1
			}
			i = end + 1
			continue
		default:
			if c == b {
				return i
			}
		}
		i++
	}
	return -1
}

// unquoteJS decodes a single-, double-, or backtick-quoted JavaScript string
// literal, translating the escape sequences that appear in inlined GraphQL
// selection-set snippets (\n, \t, \r, \\, and escaped quotes). It returns
// ok=false when expr is not a single quoted literal (e.g. a bare identifier or
// text with an unescaped closing quote before the end).
func unquoteJS(expr string) (string, bool) {
	if len(expr) < 2 {
		return "", false
	}
	q := expr[0]
	if q != '\'' && q != '"' && q != '`' {
		return "", false
	}
	if expr[len(expr)-1] != q {
		return "", false
	}
	body := expr[1 : len(expr)-1]
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\\' && i+1 < len(body) {
			i++
			switch body[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			case '"':
				b.WriteByte('"')
			case '`':
				b.WriteByte('`')
			default:
				b.WriteByte('\\')
				b.WriteByte(body[i])
			}
			continue
		}
		if c == q {
			return "", false
		}
		b.WriteByte(c)
	}
	return b.String(), true
}

// expandOnce replaces every currently-resolvable ${...} in doc in a single
// left-to-right pass and reports whether anything changed. Interpolations
// introduced by a substituted fragment are handled by the next pass.
func (r *resolver) expandOnce(doc string, env map[string]callValue) (string, bool) {
	changed := false
	var b strings.Builder
	i := 0
	for i < len(doc) {
		if strings.HasPrefix(doc[i:], "${") {
			if close := matchInterp(doc, i+1); close >= 0 {
				expr := strings.TrimSpace(doc[i+2 : close])
				if val, ok := r.evalExpr(expr, env); ok {
					b.WriteString(val)
					changed = true
					i = close + 1
					continue
				}
			}
		}
		b.WriteByte(doc[i])
		i++
	}
	return b.String(), changed
}

// evalExpr evaluates a single interpolation expression: a bare identifier
// (parameter or fragment), a quoted empty string, or a
// `cond ? THEN : ELSE` ternary whose condition is a boolean parameter. Returns
// ok=false when the expression cannot be resolved.
func (r *resolver) evalExpr(expr string, env map[string]callValue) (string, bool) {
	if q := indexTopLevel(expr, '?'); q >= 0 {
		colon := indexTopLevel(expr[q+1:], ':')
		if colon < 0 {
			return "", false
		}
		cond := strings.TrimSpace(expr[:q])
		thenExpr := strings.TrimSpace(expr[q+1 : q+1+colon])
		elseExpr := strings.TrimSpace(expr[q+1+colon+1:])
		v, ok := env[cond]
		if !ok || !v.isBool {
			return "", false
		}
		if v.b {
			return r.evalOperand(thenExpr, env)
		}
		return r.evalOperand(elseExpr, env)
	}
	return r.evalOperand(expr, env)
}

// evalOperand resolves a bound parameter, a fragment identifier, or a quoted
// string literal (single, double, or backtick — including multi-line and the
// empty string).
func (r *resolver) evalOperand(expr string, env map[string]callValue) (string, bool) {
	if v, ok := env[expr]; ok && !v.isBool {
		return v.text, true
	}
	if frag, ok := r.fragments[expr]; ok {
		return frag, true
	}
	if s, ok := unquoteJS(expr); ok {
		return s, true
	}
	return "", false
}

// evalCall evaluates a builder call expression such as
// `getSamplingRulesQuery(NOISY_OPERATION_FIELDS_V126)` into a resolved GraphQL
// document. Returns ok=false when the callee is unknown.
func (r *resolver) evalCall(expr string) (string, bool) {
	open := strings.IndexByte(expr, '(')
	if open < 0 || !strings.HasSuffix(strings.TrimSpace(expr), ")") {
		return "", false
	}
	name := strings.TrimSpace(expr[:open])
	b, ok := r.builders[name]
	if !ok {
		return "", false
	}
	argsRaw := strings.TrimSpace(expr[open+1:])
	argsRaw = strings.TrimSuffix(argsRaw, ")")
	var args []string
	if strings.TrimSpace(argsRaw) != "" {
		for _, a := range strings.Split(argsRaw, ",") {
			args = append(args, strings.TrimSpace(a))
		}
	}

	env := map[string]callValue{}
	for i, p := range b.Params {
		switch {
		case i < len(args):
			env[p.Name] = r.argValue(args[i])
		case p.HasDefault:
			env[p.Name] = r.argValue(p.Default)
		}
	}
	return r.resolveEnv(b.Body, env), true
}

// argValue converts a call argument (a boolean literal or a fragment
// identifier) into a bound value.
func (r *resolver) argValue(arg string) callValue {
	switch arg {
	case "true":
		return callValue{isBool: true, b: true}
	case "false":
		return callValue{isBool: true, b: false}
	}
	if s, ok := unquoteJS(arg); ok {
		return callValue{text: s}
	}
	if frag, ok := r.fragments[arg]; ok {
		return callValue{text: r.resolve(frag)}
	}
	// Unknown identifier: pass through as literal text so it is visible.
	return callValue{text: arg}
}

// warnUnresolvedOps prints a warning for any interpolation left unresolved in
// the final documents (a missing fragment/builder or an unsupported shape).
func warnUnresolvedOps(central []gqlOp, queryRemote, mutationRemote []remoteOp) {
	warn := func(name, doc string) {
		if idx := strings.Index(doc, "${"); idx >= 0 {
			snippet := doc[idx:]
			if len(snippet) > 60 {
				snippet = snippet[:60]
			}
			fmt.Fprintf(os.Stderr, "gen-graphql: WARNING: %s has unresolved interpolation near %q\n", name, snippet)
		}
	}
	for _, op := range central {
		warn(op.Name, op.Doc)
	}
	for _, ops := range [][]remoteOp{queryRemote, mutationRemote} {
		for _, op := range ops {
			for _, variants := range op.Variants {
				for _, doc := range variants {
					warn(op.Name, doc)
				}
			}
		}
	}
}

// extractDeclarations scans a file's source (with comments already stripped)
// and returns every recognised top-level export, resolving interpolations
// through r as documents are extracted.
func extractDeclarations(src string, r *resolver) ([]gqlOp, []remoteOp) {
	var gqls []gqlOp
	var remotes []remoteOp

	for {
		m := exportConstRe.FindStringSubmatchIndex(src)
		if m == nil {
			break
		}
		name := src[m[2]:m[3]]
		var typeAnnot string
		if m[4] >= 0 {
			typeAnnot = src[m[4]:m[5]]
		}
		// Body starts immediately after the `=` we matched.
		body := src[m[1]:]

		switch {
		case typeAnnot == "VersionedRemoteFetch":
			op, consumed := parseVersionedRemoteFetch(name, body, r)
			if op != nil {
				remotes = append(remotes, *op)
			}
			src = body[consumed:]
		case strings.HasPrefix(strings.TrimSpace(body), "gql`"):
			doc, consumed := parseGqlTemplate(body)
			if doc != "" {
				gqls = append(gqls, gqlOp{Name: name, Doc: r.resolve(doc)})
			}
			src = body[consumed:]
		default:
			// Unrecognised RHS shape; advance past the `=` to avoid a loop.
			src = body
		}
	}

	return gqls, remotes
}

// parseGqlTemplate consumes a gql-tagged template literal starting from the
// first non-space character of body and returns the doc plus the number of
// bytes consumed (relative to body).
func parseGqlTemplate(body string) (string, int) {
	idx := strings.Index(body, "gql`")
	if idx < 0 {
		return "", 0
	}
	start := idx + len("gql`")
	end := strings.IndexByte(body[start:], '`')
	if end < 0 {
		return "", len(body)
	}
	doc := strings.TrimSpace(body[start : start+end])
	return doc, start + end + 1
}

// parseVersionedRemoteFetch parses an object literal of the form:
//
//	{
//	  [PlatformType.K8s]: { 'v1.20': `...`, 'v1.22': `...` },
//	  [PlatformType.Vm]:  { 'v0.1':  `...` },
//	}
//
// and returns a remoteOp plus bytes consumed. Variant values may be template
// literals or arrow-function-builder calls; both are resolved through r.
func parseVersionedRemoteFetch(name, body string, r *resolver) (*remoteOp, int) {
	openIdx := strings.IndexByte(body, '{')
	if openIdx < 0 {
		return nil, len(body)
	}
	// Find the matching closing brace at depth 0, ignoring braces inside
	// backticks.
	end := -1
	depth := 0
	inTpl := false
	for i := openIdx; i < len(body); i++ {
		c := body[i]
		if c == '`' {
			inTpl = !inTpl
			continue
		}
		if inTpl {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	if end < 0 {
		return nil, len(body)
	}
	objLit := body[openIdx : end+1]

	op := &remoteOp{
		Name:     name,
		Variants: map[string]map[string]string{},
	}

	// Walk the object literal looking for [PlatformType.X]: { ... } sections.
	// For each section, extract its 'vX.Y': `...` entries.
	for {
		match := platformKeyRe.FindStringSubmatchIndex(objLit)
		if match == nil {
			break
		}
		plat := objLit[match[2]:match[3]]
		// Section body starts at the `{` we matched (match[1]-1 since the regex
		// consumes through `{`).
		braceStart := match[1] - 1
		sectionEnd := matchBrace(objLit, braceStart)
		if sectionEnd < 0 {
			break
		}
		section := objLit[braceStart : sectionEnd+1]
		variants := parseVariantsBlock(section, r)
		if len(variants) > 0 {
			op.Variants[plat] = variants
		}
		objLit = objLit[sectionEnd+1:]
	}

	if len(op.Variants) == 0 {
		return nil, end + 1
	}
	return op, end + 1
}

// matchBrace returns the index of the closing `}` matching the `{` at
// position openIdx in s, or -1 if unbalanced. Backtick template literals are
// skipped because they may contain stray braces.
func matchBrace(s string, openIdx int) int {
	depth := 0
	inTpl := false
	for i := openIdx; i < len(s); i++ {
		c := s[i]
		if c == '`' {
			inTpl = !inTpl
			continue
		}
		if inTpl {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseVariantsBlock pulls 'vX.Y': <value> pairs out of a single platform
// section. A value is either a template literal (`...`) or an arrow-function
// builder call (fnName(args)); both are resolved to a complete GraphQL
// document through r.
func parseVariantsBlock(section string, r *resolver) map[string]string {
	out := map[string]string{}
	idx := 0
	for {
		m := variantKeyRe.FindStringSubmatchIndex(section[idx:])
		if m == nil {
			break
		}
		ver := strings.ToLower(section[idx+m[2] : idx+m[3]])
		// Position just past the matched `'vX.Y':` key.
		valStart := idx + m[1]
		rest := section[valStart:]
		trimmed := strings.TrimLeft(rest, " \t\r\n")
		lead := len(rest) - len(trimmed)

		if strings.HasPrefix(trimmed, "`") {
			// Template literal value.
			bodyStart := valStart + lead + 1
			bodyEnd := strings.IndexByte(section[bodyStart:], '`')
			if bodyEnd < 0 {
				break
			}
			doc := strings.TrimSpace(section[bodyStart : bodyStart+bodyEnd])
			out[ver] = r.resolve(doc)
			idx = bodyStart + bodyEnd + 1
			continue
		}

		// Otherwise treat the value as a builder call expression, reading up to
		// the matching close paren.
		callAbs := valStart + lead
		open := strings.IndexByte(section[callAbs:], '(')
		if open < 0 {
			idx = callAbs
			continue
		}
		closeRel := matchParen(section[callAbs+open:])
		if closeRel < 0 {
			break
		}
		callExpr := strings.TrimSpace(section[callAbs : callAbs+open+closeRel+1])
		if doc, ok := r.evalCall(callExpr); ok {
			out[ver] = doc
		}
		idx = callAbs + open + closeRel + 1
	}
	return out
}

// matchParen returns the index (relative to s[0], which must be '(') of the
// matching ')'. Parens inside backtick template literals are ignored.
func matchParen(s string) int {
	depth := 0
	inTpl := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '`':
			inTpl = !inTpl
		case '(':
			if !inTpl {
				depth++
			}
		case ')':
			if !inTpl {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

// stripComments removes `// line` and `/* block */` comments from TS source.
// It is template-literal aware so it does not eat backticks inside strings.
func stripComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	inTpl := false
	for i < len(src) {
		c := src[i]
		if c == '`' {
			inTpl = !inTpl
			b.WriteByte(c)
			i++
			continue
		}
		if !inTpl && c == '/' && i+1 < len(src) {
			if src[i+1] == '/' {
				j := strings.IndexByte(src[i:], '\n')
				if j < 0 {
					return b.String()
				}
				b.WriteByte('\n')
				i += j + 1
				continue
			}
			if src[i+1] == '*' {
				j := strings.Index(src[i+2:], "*/")
				if j < 0 {
					return b.String()
				}
				i += j + 4
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// filterVariants normalises a list of remote operations against per-platform
// minimum versions:
//
//   - Variants strictly below the minimum are removed BUT the highest of them
//     (the "baseline" that historically applied to clients at min version) is
//     promoted and re-keyed to the minimum. This preserves the UI semantic
//     where Pick(want) returns the highest variant <= want, while presenting a
//     clean generated file where no version below `min` ever appears.
//   - Variants at or above the minimum are kept as-is.
//   - Operations that end up with no variants for any platform are dropped.
func filterVariants(ops []remoteOp, mins map[string]version.Version) []remoteOp {
	out := make([]remoteOp, 0, len(ops))
	for _, op := range ops {
		filtered := remoteOp{Name: op.Name, Variants: map[string]map[string]string{}}
		for plat, variants := range op.Variants {
			min, hasMin := mins[plat]

			// Bucket the original variants by version after parsing.
			parsed := make(map[version.Version]string, len(variants))
			for vstr, doc := range variants {
				v, err := version.Parse(vstr)
				if err != nil {
					continue
				}
				parsed[v] = doc
			}

			kept := map[string]string{}
			var baselineVer version.Version
			var baselineDoc string
			haveBaseline := false

			for v, doc := range parsed {
				switch {
				case !hasMin || v.GTE(min):
					kept[v.String()] = doc
				default:
					// Below min: candidate baseline (the highest sub-min variant wins).
					if !haveBaseline || v.GTE(baselineVer) {
						baselineVer = v
						baselineDoc = doc
						haveBaseline = true
					}
				}
			}

			// If we promoted a baseline and there is no explicit variant at min,
			// install the baseline at min so Pick(min) succeeds.
			if haveBaseline {
				if _, exists := kept[min.String()]; !exists {
					kept[min.String()] = baselineDoc
				}
			}

			if len(kept) > 0 {
				filtered.Variants[plat] = kept
			}
		}
		if len(filtered.Variants) > 0 {
			out = append(out, filtered)
		}
	}
	return out
}

func dedupSortGql(ops *[]gqlOp) {
	seen := map[string]bool{}
	deduped := (*ops)[:0]
	for _, op := range *ops {
		if seen[op.Name] {
			continue
		}
		seen[op.Name] = true
		deduped = append(deduped, op)
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Name < deduped[j].Name })
	*ops = deduped
}

func dedupSortRemote(ops *[]remoteOp) {
	seen := map[string]bool{}
	deduped := (*ops)[:0]
	for _, op := range *ops {
		if seen[op.Name] {
			continue
		}
		seen[op.Name] = true
		deduped = append(deduped, op)
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Name < deduped[j].Name })
	*ops = deduped
}

// templateData* are the Render targets for text/template.

type centralData struct {
	Ops []gqlOp
}

type remoteData struct {
	FileTag string
	Ops     []remoteRender
}

type remoteRender struct {
	Name      string
	Platforms []platformRender
}

type platformRender struct {
	Name     string // K8s, Vm
	Variants []variantRender
}

type variantRender struct {
	Version string // canonical "v1.20"
	Doc     string
}

const fileHeader = `// Code generated by tools/gen-graphql. DO NOT EDIT.
//
// Source: central-ui/src/graphql/{queries,mutations}/**/*.ts
// Regenerate with: go generate ./operations
`

const centralTemplate = fileHeader + `
package operations
{{range .Ops}}
// {{.Name}} is a central-scoped GraphQL operation. It does not vary by
// proxy version because it targets Central directly.
const {{.Name}} = ` + "`{{.Doc}}`" + `
{{end}}`

func remoteTemplate(tag string) string {
	return fileHeader + `
package operations

import (
	"github.com/odigos-io/odigos-central-client/platform"
	"github.com/odigos-io/odigos-central-client/version"
)
{{if not .Ops}}
var (
	_ = platform.K8s
	_ = version.Version{}
)
{{end}}{{range .Ops}}
// {{.Name}} is a remote (cluster-scoped) GraphQL operation. Its document
// varies by proxy platform type and Odigos version; use Operation.Pick to
// select the right variant for a given proxy.
var {{.Name}} = Operation{
	Name: "{{.Name}}",
	Variants: map[platform.Type]map[version.Version]string{
{{range .Platforms}}		platform.{{.Name}}: {
{{range .Variants}}			version.MustParse("{{.Version}}"): ` + "`{{.Doc}}`," + `
{{end}}		},
{{end}}	},
}
{{end}}
// All` + tagSuffix(tag) + ` is the registry of remote operations declared in this
// file. Compatibility-matrix tests iterate over it to assert version-floor
// invariants across the entire generated surface.
var All` + tagSuffix(tag) + ` = []*Operation{
{{range .Ops}}	&{{.Name}},
{{end}}}
`
}

func tagSuffix(tag string) string {
	switch tag {
	case "remote_queries":
		return "RemoteQueries"
	case "remote_mutations":
		return "RemoteMutations"
	default:
		return "Operations"
	}
}

// writeGoFile renders the appropriate template, gofmts the result, and writes it.
// `data` is either []gqlOp (for central) or []remoteOp (for remote files).
func writeGoFile(path, tmpl string, data any) error {
	t, err := template.New(filepath.Base(path)).Parse(tmpl)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	switch v := data.(type) {
	case []gqlOp:
		// Sanitise docs before rendering: a stray backtick inside a doc would
		// break the raw-string literal. None should appear in real GraphQL,
		// but we defensively replace.
		clean := make([]gqlOp, len(v))
		for i, op := range v {
			clean[i] = gqlOp{Name: op.Name, Doc: stripBackticks(op.Doc)}
		}
		if err := t.Execute(&buf, centralData{Ops: clean}); err != nil {
			return err
		}
	case []remoteOp:
		rd := buildRemoteRender(v)
		if err := t.Execute(&buf, rd); err != nil {
			return err
		}
	default:
		return fmt.Errorf("writeGoFile: unsupported data type %T", data)
	}

	// format.Source applies gofmt, so the emitted file is gofmt-clean as-is.
	// Do not post-process the result (e.g. stripping blank lines), or the
	// output will no longer satisfy `gofmt -l`.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Save a debug copy so a parse failure is diagnosable.
		_ = os.WriteFile(path+".broken", buf.Bytes(), 0o644)
		return fmt.Errorf("gofmt %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0o644)
}

func buildRemoteRender(ops []remoteOp) remoteData {
	rd := remoteData{Ops: make([]remoteRender, 0, len(ops))}
	platOrder := []string{"K8s", "Vm"}
	for _, op := range ops {
		rr := remoteRender{Name: op.Name}
		for _, p := range platOrder {
			variants, ok := op.Variants[p]
			if !ok {
				continue
			}
			vKeys := make([]string, 0, len(variants))
			for v := range variants {
				vKeys = append(vKeys, v)
			}
			// Highest first for stable, easy-to-read output.
			sort.Slice(vKeys, func(i, j int) bool {
				vi, _ := version.Parse(vKeys[i])
				vj, _ := version.Parse(vKeys[j])
				return vi.Compare(vj) > 0
			})
			pr := platformRender{Name: p}
			for _, k := range vKeys {
				pr.Variants = append(pr.Variants, variantRender{
					Version: k,
					Doc:     stripBackticks(variants[k]),
				})
			}
			rr.Platforms = append(rr.Platforms, pr)
		}
		rd.Ops = append(rd.Ops, rr)
	}
	return rd
}

func stripBackticks(s string) string { return strings.ReplaceAll(s, "`", "'") }
