package client

import (
	"context"
	"net/http"
	"net/url"
)

const groupsPath = "/api/v1/groups"

// MonitorGroup is the read shape (`GroupView`).
type MonitorGroup struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	AlertAsOne           bool           `json:"alert_as_one"`
	Alerts               []AlertBinding `json:"alerts"`
	RenotifyIntervalSecs uint32         `json:"renotify_interval_secs"`
	AlertAgain           bool           `json:"alert_again"`
	Tags                 []string       `json:"tags"`
	MemberCount          uint32         `json:"member_count"`
}

// MonitorGroupInput is the POST and the PATCH body. Terraform holds the full
// desired state, so every field travels on both.
type MonitorGroupInput struct {
	Name                 string         `json:"name"`
	AlertAsOne           bool           `json:"alert_as_one"`
	Alerts               []AlertBinding `json:"alerts"`
	RenotifyIntervalSecs uint32         `json:"renotify_interval_secs"`
	AlertAgain           bool           `json:"alert_again"`
	Tags                 []string       `json:"tags"`
}

// The PATCH reads a null list as "keep", so an empty plan must clear.
func (in MonitorGroupInput) normalized() MonitorGroupInput {
	if in.Alerts == nil {
		in.Alerts = []AlertBinding{}
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	return in
}

func groupPath(id string) string {
	return groupsPath + "/" + url.PathEscape(id)
}

func (c *Client) CreateGroup(ctx context.Context, in MonitorGroupInput) (*MonitorGroup, error) {
	var out MonitorGroup
	if err := c.do(ctx, http.MethodPost, groupsPath, in.normalized(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetGroup(ctx context.Context, id string) (*MonitorGroup, error) {
	var out MonitorGroup
	if err := c.do(ctx, http.MethodGet, groupPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListGroups returns every group in the org, empty ones included.
func (c *Client) ListGroups(ctx context.Context) ([]MonitorGroup, error) {
	var out []MonitorGroup
	if err := c.do(ctx, http.MethodGet, groupsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdateGroup(ctx context.Context, id string, in MonitorGroupInput) (*MonitorGroup, error) {
	var out MonitorGroup
	if err := c.do(ctx, http.MethodPatch, groupPath(id), in.normalized(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteGroup leaves the group's monitors in place, ungrouped.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, groupPath(id), nil, nil)
}
