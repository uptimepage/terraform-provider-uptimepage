package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const targetsPath = "/api/v1/targets"

// TargetPage is the paginated list envelope.
type TargetPage struct {
	Items   []Target `json:"items"`
	Limit   uint32   `json:"limit"`
	Offset  uint32   `json:"offset"`
	HasMore bool     `json:"has_more"`
}

// ListParams are the optional /targets list filters. Zero values are omitted.
type ListParams struct {
	Limit   uint32
	Offset  uint32
	Tag     string
	Enabled *bool
}

func (c *Client) CreateTarget(ctx context.Context, in NewTarget) (*Target, error) {
	var out Target
	if err := c.do(ctx, http.MethodPost, targetsPath, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetTarget(ctx context.Context, id string) (*Target, error) {
	var out Target
	if err := c.do(ctx, http.MethodGet, targetsPath+"/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateTarget(ctx context.Context, id string, in TargetUpdate) (*Target, error) {
	// Non-nil so an empty plan clears rather than a null being read as "keep".
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if in.Alerts == nil {
		in.Alerts = []AlertBinding{}
	}
	var out Target
	if err := c.do(ctx, http.MethodPatch, targetsPath+"/"+url.PathEscape(id), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteTarget(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, targetsPath+"/"+url.PathEscape(id), nil, nil)
}

// regionsPath is the per-target region sub-resource.
func regionsPath(id string) string {
	return targetsPath + "/" + url.PathEscape(id) + "/regions"
}

// GetTargetRegions returns the region set currently assigned to the target
// (requires scope targets:read). A missing target surfaces as a 404 *APIError.
func (c *Client) GetTargetRegions(ctx context.Context, id string) ([]string, error) {
	var out TargetRegions
	if err := c.do(ctx, http.MethodGet, regionsPath(id), nil, &out); err != nil {
		return nil, err
	}
	return out.Regions, nil
}

// SetTargetRegions replaces the target's region set and returns the stored set
// (requires scope targets:write). regions must be non-empty and every id must
// name an enabled region, else the server responds 422 REGION_INVALID.
func (c *Client) SetTargetRegions(ctx context.Context, id string, regions []string) ([]string, error) {
	var out TargetRegions
	if err := c.do(ctx, http.MethodPut, regionsPath(id), TargetRegions{Regions: regions}, &out); err != nil {
		return nil, err
	}
	return out.Regions, nil
}

func (c *Client) ListTargets(ctx context.Context, p ListParams) (*TargetPage, error) {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.FormatUint(uint64(p.Limit), 10))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.FormatUint(uint64(p.Offset), 10))
	}
	if p.Tag != "" {
		q.Set("tag", p.Tag)
	}
	if p.Enabled != nil {
		q.Set("enabled", strconv.FormatBool(*p.Enabled))
	}
	path := targetsPath
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out TargetPage
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	return &out, nil
}
