package odigos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/odigos-io/odigos-central-client/logger"
)

type OdigosClient struct {
	url        string
	httpClient *http.Client
	token      string
	logger     logger.CustomLogger
}

type ClientConfig struct {
	Hostname   string
	HTTPClient *http.Client
	Username   string
	Password   string
	Insecure   bool
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func NewClient(ctx context.Context, config ClientConfig, logOptions ...logger.LoggingOption) (*OdigosClient, error) {
	logConfig := logger.SetLogConfig(logOptions)
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}

	query, variables := authQuery(config)

	var url string
	if !config.Insecure {
		url = fmt.Sprintf("https://%s/graphql", config.Hostname)
	} else {
		url = fmt.Sprintf("http://%s/graphql", config.Hostname)
	}

	client := &OdigosClient{
		url:        url,
		httpClient: config.HTTPClient,
		token:      "",
		logger:     logConfig.Logger,
	}
	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}
	err := client.authenticate(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *OdigosClient) authenticate(ctx context.Context, reqBody graphQLRequest) error {
	graphqlResp, err := c.execute(ctx, reqBody)
	if err != nil {
		return err
	}
	var authResponse authResponse
	if err := json.Unmarshal(graphqlResp, &authResponse); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}
	if len(authResponse.Errors) > 0 {
		msgs := make([]string, len(authResponse.Errors))
		for i, e := range authResponse.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("sign-in failed: %s", strings.Join(msgs, "; "))
	}
	if authResponse.Data.SignIn == nil || authResponse.Data.SignIn.AccessToken == "" {
		return fmt.Errorf("sign-in failed: no access token returned (invalid credentials or server error)")
	}
	c.token = authResponse.Data.SignIn.AccessToken

	return nil
}

func authQuery(config ClientConfig) (query string, variables map[string]interface{}) {
	variables = map[string]interface{}{
		"email":    config.Username,
		"password": config.Password,
	}
	return SignInMutation, variables
}

func (c *OdigosClient) GetComputePlatforms(ctx context.Context) ([]ComputePlatform, error) {
	reqBody := graphQLRequest{
		Query:     GetComputePlatformsQuery,
		Variables: nil,
	}
	graphqlResp, err := c.execute(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	var resp computePlatformsResponse
	if err := json.Unmarshal(graphqlResp, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse compute platforms response: %w", err)
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("compute platforms query failed: %s", strings.Join(msgs, "; "))
	}
	return resp.Data.ComputePlatforms, nil
}

func (c *OdigosClient) GetNamespaces(ctx context.Context, proxyID string) ([]K8sActualNamespace, error) {
	resp, err := fetchRemote[namespacesResponse](ctx, c, proxyID, GetNamespacesQuery, map[string]any{})
	if err != nil {
		return nil, err
	}

	return resp.Data.ComputePlatform.K8SActualNamespaces, nil
}

func (c *OdigosClient) GetSources(ctx context.Context, proxyID string) ([]Source, error) {
	resp, err := fetchRemote[sourcesResponse](ctx, c, proxyID, GetSourcesQuery, map[string]any{})
	if err != nil {
		return nil, err
	}

	return resp.Data.ComputePlatform.Sources, nil
}

func (c *OdigosClient) GetSource(ctx context.Context, proxyID string, sourceID SourceId) (Source, error) {
	resp, err := fetchRemote[sourceResponse](ctx, c, proxyID, GetSourceQuery, map[string]any{"sourceId": sourceID})
	if err != nil {
		return Source{}, err
	}

	return resp.Data.ComputePlatform.Source, nil
}

func (c *OdigosClient) GetNamespace(ctx context.Context, proxyID string, namespaceName string) (K8sActualNamespaceDetail, error) {
	resp, err := fetchRemote[namespaceResponse](ctx, c, proxyID, GetNamespaceQuery, map[string]any{"namespaceName": namespaceName})
	if err != nil {
		return K8sActualNamespaceDetail{}, err
	}

	return resp.Data.ComputePlatform.K8sActualNamespace, nil
}

func (c *OdigosClient) PersistSources(ctx context.Context, proxyID string, sources []SourceInput) (bool, error) {
	resp, err := fetchRemote[persistSourcesResponse](ctx, c, proxyID, PersistSourcesMutation, map[string]any{"sources": sources})
	if err != nil {
		return false, err
	}
	return resp.Data.PersistK8sSources, nil
}

func (c *OdigosClient) PersistNamespaceSources(ctx context.Context, proxyID string, namespaces []NamespaceInput) (bool, error) {
	resp, err := fetchRemote[persistNamespaceSourcesResponse](ctx, c, proxyID, PersistNamespaceSourcesMutation, map[string]any{"namespaces": namespaces})
	if err != nil {
		return false, err
	}
	return resp.Data.PersistK8sNamespaces, nil
}

func (c *OdigosClient) execute(ctx context.Context, reqBody graphQLRequest) ([]byte, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	c.logger.Debug(ctx, "graphql request", "method", req.Method, "url", c.url, "body", sanitizeBody(reqBody))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug(ctx, "graphql response", "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql: server returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
