package central

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/odigos-io/odigos-central-client/logger"
	"github.com/odigos-io/odigos-central-client/operations"
)

// remoteFetcher abstracts the transport dependency a ProxyClient needs. It is
// a seam: a proxy is given one rather than reaching back into its parent,
// which keeps ProxyClient testable in isolation and leaves room to wrap the
// transport with retries or middleware later. *transport satisfies it.
type remoteFetcher interface {
	remoteFetch(ctx context.Context, opName, proxyID, query string, variables map[string]any, out any) error
}

// transport handles raw HTTP communication with Central. It is intentionally
// dependency-free (stdlib only) so the client stays small and easy to vendor.
type transport struct {
	url        string
	httpClient *http.Client
	token      string
	logger     logger.CustomLogger
}

func newTransport(url string, hc *http.Client, log logger.CustomLogger) *transport {
	return &transport{url: url, httpClient: hc, logger: log}
}

// graphQLRequest is the wire envelope for every request we send.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// graphqlError matches the items in a GraphQL response's "errors" array.
type graphqlError struct {
	Message    string                 `json:"message"`
	Path       []any                  `json:"path,omitempty"`
	Locations  []graphqlErrorLocation `json:"locations,omitempty"`
	Extensions map[string]any         `json:"extensions,omitempty"`
}

type graphqlErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// toGraphQLError converts a decoded "errors" array into the public,
// structured *GraphQLError. It is the single place that maps wire errors to
// the exported type so decodeData and remoteFetch stay consistent.
func toGraphQLError(op string, errs []graphqlError) *GraphQLError {
	details := make([]GraphQLErrorDetail, len(errs))
	for i, e := range errs {
		var locs []GraphQLErrorLocation
		if len(e.Locations) > 0 {
			locs = make([]GraphQLErrorLocation, len(e.Locations))
			for j, l := range e.Locations {
				locs[j] = GraphQLErrorLocation{Line: l.Line, Column: l.Column}
			}
		}
		details[i] = GraphQLErrorDetail{
			Message:    e.Message,
			Path:       e.Path,
			Locations:  locs,
			Extensions: e.Extensions,
		}
	}
	return &GraphQLError{Op: op, Errors: details}
}

// errorEnvelope is decoded purely to surface "errors" while leaving "data"
// alone for the caller's typed unmarshal.
type errorEnvelope struct {
	Errors []graphqlError `json:"errors"`
}

// execute runs a GraphQL operation and returns the raw response bytes,
// allowing the caller to unmarshal into a typed struct of their choice.
//
// The opName argument is included in error messages and logs. It is not
// otherwise inspected.
func (t *transport) execute(ctx context.Context, opName string, body graphQLRequest) ([]byte, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %q request: %w", opName, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("build %q request: %w", opName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}

	if t.logger != nil {
		t.logger.Debug(ctx, "graphql request", "op", opName, "url", t.url, "body", sanitizeBody(body))
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute %q: %w", opName, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %q response: %w", opName, err)
	}

	if t.logger != nil {
		t.logger.Debug(ctx, "graphql response", "op", opName, "status", resp.StatusCode, "body", sanitizeResponse(respBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql %q: server returned status %d: %s", opName, resp.StatusCode, string(respBytes))
	}

	return respBytes, nil
}

// decodeData runs `execute` and decodes the `data` field of the response into
// `out`. If the response carries a non-empty "errors" array, a structured
// *GraphQLError is returned and `out` is left untouched.
func (t *transport) decodeData(ctx context.Context, opName string, body graphQLRequest, out any) error {
	raw, err := t.execute(ctx, opName, body)
	if err != nil {
		return err
	}

	var env errorEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("graphql %q: parse response envelope: %w", opName, err)
	}
	if len(env.Errors) > 0 {
		return toGraphQLError(opName, env.Errors)
	}

	if out == nil {
		return nil
	}

	var withData struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &withData); err != nil {
		return fmt.Errorf("graphql %q: parse data field: %w", opName, err)
	}
	if len(withData.Data) == 0 || string(withData.Data) == "null" {
		return fmt.Errorf("graphql %q: empty data in response", opName)
	}
	if err := json.Unmarshal(withData.Data, out); err != nil {
		return fmt.Errorf("graphql %q: decode data: %w", opName, err)
	}
	return nil
}

// remoteFetch wraps a cluster-scoped GraphQL document inside Central's
// remoteFetch query. The remoteFetch field returns the inner GraphQL response
// as a JSON-encoded string; we unwrap it transparently and decode the inner
// `data` field into `out`.
//
// The `proxyID`, `query`, and `variables` parameters mirror the GraphQL
// arguments of the remoteFetch operation.
func (t *transport) remoteFetch(
	ctx context.Context,
	opName, proxyID, query string,
	variables map[string]any,
	out any,
) error {
	if variables == nil {
		variables = map[string]any{}
	}
	body := graphQLRequest{
		Query: operations.REMOTE_FETCH,
		Variables: map[string]any{
			"proxyID":   proxyID,
			"query":     query,
			"variables": variables,
		},
	}

	raw, err := t.execute(ctx, opName, body)
	if err != nil {
		return err
	}

	data, err := unwrapRemoteFetch(opName, raw)
	if err != nil {
		return err
	}

	if out == nil {
		return nil
	}
	if len(data) == 0 || string(data) == "null" {
		return fmt.Errorf("graphql %q: inner data is null", opName)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("graphql %q: decode inner data: %w", opName, err)
	}
	return nil
}

// unwrapRemoteFetch peels Central's remoteFetch envelope: it validates the
// outer GraphQL response, unwraps the JSON-encoded inner payload, surfaces
// GraphQL errors from either layer as a *GraphQLError, and returns the inner
// `data` message for the caller to decode.
func unwrapRemoteFetch(opName string, raw []byte) (json.RawMessage, error) {
	// Outer response: { data: { remoteFetch: "<inner json string>" }, errors: [...] }
	var outer struct {
		Data struct {
			RemoteFetch json.RawMessage `json:"remoteFetch"`
		} `json:"data"`
		Errors []graphqlError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("graphql %q: parse outer envelope: %w", opName, err)
	}
	if len(outer.Errors) > 0 {
		return nil, toGraphQLError(opName, outer.Errors)
	}
	if len(outer.Data.RemoteFetch) == 0 || string(outer.Data.RemoteFetch) == "null" {
		return nil, fmt.Errorf("graphql %q: remoteFetch returned null (proxy unreachable?)", opName)
	}

	// remoteFetch is a JSON-encoded string; unwrap it.
	var innerJSON string
	if err := json.Unmarshal(outer.Data.RemoteFetch, &innerJSON); err != nil {
		return nil, fmt.Errorf("graphql %q: unwrap remoteFetch payload: %w", opName, err)
	}
	if innerJSON == "" {
		return nil, fmt.Errorf("graphql %q: remoteFetch returned empty string", opName)
	}

	// Inner response: { type: "graphql_response", data: { ... }, request_id: "..." }
	// where data may itself contain `errors` for HTTP-200 GraphQL failures.
	var inner struct {
		Type      string          `json:"type"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(innerJSON), &inner); err != nil {
		return nil, fmt.Errorf("graphql %q: parse inner remote payload: %w", opName, err)
	}
	if len(inner.Data) > 0 {
		var dataErrs struct {
			Errors []graphqlError `json:"errors"`
		}
		if err := json.Unmarshal(inner.Data, &dataErrs); err == nil && len(dataErrs.Errors) > 0 {
			return nil, toGraphQLError(opName, dataErrs.Errors)
		}
	}
	return inner.Data, nil
}

// sensitiveKeys is the redaction list applied to request variables when
// logging request bodies. Both a login password and a 2FA code are secrets.
var sensitiveKeys = map[string]bool{
	"password": true,
	"code":     true,
}

func sanitizeBody(reqBody graphQLRequest) string {
	safe := graphQLRequest{
		Query:     reqBody.Query,
		Variables: make(map[string]any, len(reqBody.Variables)),
	}
	for k, v := range reqBody.Variables {
		if sensitiveKeys[strings.ToLower(k)] {
			safe.Variables[k] = "****"
		} else {
			safe.Variables[k] = v
		}
	}
	data, err := json.Marshal(safe)
	if err != nil {
		return "<failed to marshal sanitized body>"
	}
	return string(data)
}

// sensitiveResponseKeyRe matches JSON string values for token/credential keys
// that may appear in a GraphQL response (e.g. the SignIn payload). Their values
// are redacted before the response body is logged.
var sensitiveResponseKeyRe = regexp.MustCompile(`("(?:accessToken|idToken|token|generatedPassword)"\s*:\s*)"[^"]*"`)

// sanitizeResponse redacts token/credential values from a raw response body so
// they never reach debug logs.
func sanitizeResponse(raw []byte) string {
	return string(sensitiveResponseKeyRe.ReplaceAll(raw, []byte(`${1}"****"`)))
}
