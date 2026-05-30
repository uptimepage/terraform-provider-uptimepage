// Package client is the UptimePage /api/v1 transport. It turns typed Go calls
// into authenticated HTTP requests and decodes responses + the error envelope.
// It imports nothing from Terraform and is unit-testable against httptest.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultEndpoint = "https://uptimepage.dev"

// Client is safe for concurrent use: it holds only immutable config plus an
// *http.Client (itself concurrency-safe), so there is nothing to lock.
type Client struct {
	endpoint   string // base, no trailing slash
	token      string
	httpClient *http.Client
}

// New builds a Client. An empty endpoint falls back to the public default; a
// nil httpClient falls back to http.DefaultClient.
func New(endpoint, token string, httpClient *http.Client) *Client {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

// do issues one request. body is JSON-encoded when non-nil; out is JSON-decoded
// from the response when non-nil and the status is not 204. A >=400 status is
// decoded into *APIError. ctx cancellation aborts the request.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err // includes context cancellation
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return decodeAPIError(resp.StatusCode, raw)
	}

	if out != nil && resp.StatusCode != http.StatusNoContent && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// decodeAPIError turns a non-2xx body into an *APIError, always carrying the
// status even when the envelope is missing or malformed.
func decodeAPIError(status int, raw []byte) error {
	var env errorEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Error.Code != "" {
		env.Error.Status = status
		return &env.Error
	}
	return &APIError{
		Status:  status,
		Code:    "UNKNOWN",
		Message: strings.TrimSpace(string(raw)),
	}
}
