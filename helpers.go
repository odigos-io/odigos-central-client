package odigos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func fetchRemote[T any](ctx context.Context, c *OdigosClient, proxyID string, query string, variables map[string]any) (*T, error) {

	reqBody := graphQLRequest{
		Query: RemoteFetchQuery,
		Variables: map[string]interface{}{
			"proxyID":   proxyID,
			"query":     query,
			"variables": variables,
		},
	}

	graphqlResp, err := c.execute(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	return UnmarshalRemoteFetch[T](graphqlResp)
}

func UnmarshalRemoteFetch[T any](graphqlResp []byte) (*T, error) {
	if len(graphqlResp) == 0 {
		return nil, fmt.Errorf("empty response from server")
	}

	var graphQLRemoteResponse GraphQLRemoteResponse
	if err := json.Unmarshal(graphqlResp, &graphQLRemoteResponse); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	if len(graphQLRemoteResponse.Errors) > 0 {
		msgs := make([]string, len(graphQLRemoteResponse.Errors))
		for i, e := range graphQLRemoteResponse.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}

	if len(graphQLRemoteResponse.Data.RemoteFetch) == 0 ||
		string(graphQLRemoteResponse.Data.RemoteFetch) == "null" {
		return nil, fmt.Errorf("remote fetch returned null or empty response (the remote proxy may be unreachable or returned no data)")
	}

	var remoteFetchStr string
	if err := json.Unmarshal(graphQLRemoteResponse.Data.RemoteFetch, &remoteFetchStr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal remoteFetch as string: %w (raw value: %s)", err, string(graphQLRemoteResponse.Data.RemoteFetch))
	}

	if remoteFetchStr == "" {
		return nil, fmt.Errorf("remote fetch returned empty data string")
	}

	if err := checkRemoteGraphQLErrors([]byte(remoteFetchStr)); err != nil {
		return nil, err
	}

	var result T
	if err := json.Unmarshal([]byte(remoteFetchStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse remote fetch result: %w", err)
	}

	return &result, nil
}

// checkRemoteGraphQLErrors inspects the inner remote response for GraphQL errors
// that arrive with HTTP 200 status. The inner response has the structure:
// {"type":"graphql_response","data":{...},"request_id":"..."}
// where "data" contains the actual GraphQL response body, which may include "errors".
func checkRemoteGraphQLErrors(innerResp []byte) error {
	var check struct {
		Data struct {
			Errors []graphQLError `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(innerResp, &check); err != nil {
		return nil
	}
	if len(check.Data.Errors) > 0 {
		msgs := make([]string, len(check.Data.Errors))
		for i, e := range check.Data.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("remote GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	return nil
}

var sensitiveKeys = map[string]bool{
	"password": true,
}

func sanitizeBody(reqBody graphQLRequest) string {
	safe := graphQLRequest{
		Query:     reqBody.Query,
		Variables: make(map[string]interface{}, len(reqBody.Variables)),
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
