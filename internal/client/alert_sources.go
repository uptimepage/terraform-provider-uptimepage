package client

import (
	"context"
	"net/http"
	"net/url"
)

const alertSourcesPath = "/api/v1/alert-sources"

// Alert source kinds. The kind is fixed at create.
const (
	AlertSourceKindAlertmanager = "alertmanager"
	AlertSourceKindGrafana      = "grafana"
	AlertSourceKindDatadog      = "datadog"
	AlertSourceKindGeneric      = "generic"
)

// Alert source groupings: one incident per tool group, or per alert.
const (
	AlertGroupingGroup = "group"
	AlertGroupingAlert = "alert"
)

// AlertSource is the read shape (`AlertSourceView`). IngestURL and IngestToken
// come back only to a token with alert_sources:write.
type AlertSource struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	Kind                 string         `json:"kind"`
	Enabled              bool           `json:"enabled"`
	Grouping             string         `json:"grouping"`
	EscalationPolicyID   *string        `json:"escalation_policy_id"`
	Alerts               []AlertBinding `json:"alerts"`
	NotifyRecovery       bool           `json:"notify_recovery"`
	RenotifyIntervalSecs uint32         `json:"renotify_interval_secs"`
	Signed               bool           `json:"signed"`
	IngestURL            *string        `json:"ingest_url"`
	IngestToken          *string        `json:"ingest_token"`
	RoutesToNobody       bool           `json:"routes_to_nobody"`
	PlanHoldAt           *string        `json:"plan_hold_at"`
}

// NewAlertSource is the POST body. The API has no `enabled` on create: a source
// starts listening, and CreateAlertSource's caller pauses it afterwards.
type NewAlertSource struct {
	Name                 string         `json:"name"`
	Kind                 string         `json:"kind"`
	Grouping             string         `json:"grouping"`
	EscalationPolicyID   *string        `json:"escalation_policy_id,omitempty"`
	Alerts               []AlertBinding `json:"alerts"`
	NotifyRecovery       bool           `json:"notify_recovery"`
	RenotifyIntervalSecs uint32         `json:"renotify_interval_secs"`
	SigningSecret        *string        `json:"signing_secret,omitempty"`
}

// AlertSourceUpdate is the PATCH body: a nil field is left out and keeps its
// value. The route travels only when it changed, since the API checks a route
// it is sent and would refuse a rename of a source whose route was lost
// elsewhere. A policy bound or a secret set in the app stays unless sent.
type AlertSourceUpdate struct {
	Name                 *string         `json:"name,omitempty"`
	Enabled              *bool           `json:"enabled,omitempty"`
	Grouping             *string         `json:"grouping,omitempty"`
	NotifyRecovery       *bool           `json:"notify_recovery,omitempty"`
	RenotifyIntervalSecs *uint32         `json:"renotify_interval_secs,omitempty"`
	Alerts               *[]AlertBinding `json:"alerts,omitempty"`
	EscalationPolicyID   *string         `json:"escalation_policy_id,omitempty"`
	SigningSecret        *string         `json:"signing_secret,omitempty"`
}

func alertSourcePath(id string) string {
	return alertSourcesPath + "/" + url.PathEscape(id)
}

func (c *Client) CreateAlertSource(ctx context.Context, in NewAlertSource) (*AlertSource, error) {
	var out AlertSource
	if err := c.do(ctx, http.MethodPost, alertSourcesPath, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAlertSource(ctx context.Context, id string) (*AlertSource, error) {
	var out AlertSource
	if err := c.do(ctx, http.MethodGet, alertSourcePath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAlertSource(ctx context.Context, id string, in AlertSourceUpdate) (*AlertSource, error) {
	var out AlertSource
	if err := c.do(ctx, http.MethodPatch, alertSourcePath(id), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAlertSource stops its URL at once and resolves the incidents it holds
// open.
func (c *Client) DeleteAlertSource(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, alertSourcePath(id), nil, nil)
}
