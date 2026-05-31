package client

import (
	"context"
	"net/http"
	"net/url"
)

const statusPagesPath = "/api/v1/status-pages"

// StatusPageStyles is the set of accepted public_style values, mirrored from
// the API's CHECK list. Used by the resource schema's OneOf validator.
var StatusPageStyles = []string{
	"default", "classic", "terminal", "winter", "dark", "night", "dim",
	"nord", "dracula", "corporate", "light", "cupcake", "cyberpunk", "synthwave",
}

// StatusPage is the read shape (`StatusPageView`). Branding fields are flattened
// onto the page; logo_url / status_url are server-derived and read-only.
type StatusPage struct {
	ID                string  `json:"id"`
	Slug              string  `json:"slug"`
	Name              string  `json:"name"`
	Enabled           bool    `json:"enabled"`
	PublicDisplayName *string `json:"public_display_name,omitempty"`
	PublicAbout       *string `json:"public_about,omitempty"`
	PublicBrandColor  *string `json:"public_brand_color,omitempty"`
	PublicStyle       string  `json:"public_style"`
	// Raw override (nil = inherit the deployment default); round-trips the
	// tri-state. ShowPoweredBy is the resolved value, for reference only.
	PublicShowPoweredBy *bool   `json:"public_show_powered_by,omitempty"`
	ShowPoweredBy       bool    `json:"show_powered_by"`
	LogoURL             *string `json:"logo_url,omitempty"`
	StatusURL           *string `json:"status_url,omitempty"`
}

// NewStatusPage is the POST body. Branding is set via a follow-up PATCH, so only
// identity + enabled travel here.
type NewStatusPage struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// StatusPageUpdate is the PATCH body, sent as full desired state. Branding is a
// nested block that replaces the page's display branding wholesale.
type StatusPageUpdate struct {
	Name     string         `json:"name"`
	Slug     string         `json:"slug"`
	Enabled  bool           `json:"enabled"`
	Branding StatusBranding `json:"branding"`
}

// StatusBranding is the page's display branding. A nil pointer marshals to JSON
// null, which clears the field (display_name/about/brand_color) or applies the
// server default (style/show_powered_by) — Terraform always sends full state.
type StatusBranding struct {
	PublicDisplayName   *string `json:"public_display_name"`
	PublicAbout         *string `json:"public_about"`
	PublicBrandColor    *string `json:"public_brand_color"`
	PublicStyle         *string `json:"public_style"`
	PublicShowPoweredBy *bool   `json:"public_show_powered_by"`
}

func (c *Client) CreateStatusPage(ctx context.Context, in NewStatusPage) (*StatusPage, error) {
	var out StatusPage
	if err := c.do(ctx, http.MethodPost, statusPagesPath, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetStatusPage(ctx context.Context, id string) (*StatusPage, error) {
	var out StatusPage
	if err := c.do(ctx, http.MethodGet, statusPagesPath+"/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateStatusPage(ctx context.Context, id string, in StatusPageUpdate) (*StatusPage, error) {
	var out StatusPage
	if err := c.do(ctx, http.MethodPatch, statusPagesPath+"/"+url.PathEscape(id), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteStatusPage(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, statusPagesPath+"/"+url.PathEscape(id), nil, nil)
}

// --- components (the monitors curated onto a page) ---

// StatusPageComponent is the read shape of one curated monitor. MonitorName is
// the operator-side target name, returned for reference (read-only).
type StatusPageComponent struct {
	TargetID          string  `json:"target_id"`
	MonitorName       string  `json:"monitor_name"`
	PublicName        *string `json:"public_name,omitempty"`
	PublicDescription *string `json:"public_description,omitempty"`
	PublicGroup       *string `json:"public_group,omitempty"`
	SortOrder         int     `json:"sort_order"`
}

// NewStatusPageComponent is the POST body for adding a monitor to a page.
type NewStatusPageComponent struct {
	TargetID          string  `json:"target_id"`
	PublicName        *string `json:"public_name,omitempty"`
	PublicDescription *string `json:"public_description,omitempty"`
	PublicGroup       *string `json:"public_group,omitempty"`
	SortOrder         int     `json:"sort_order"`
}

// StatusPageComponentUpdate is the PATCH body for a component's curation. The
// string pointers are sent unconditionally (no omitempty): a nil marshals to
// JSON null, which clears the override; Terraform always sends full state.
type StatusPageComponentUpdate struct {
	PublicName        *string `json:"public_name"`
	PublicDescription *string `json:"public_description"`
	PublicGroup       *string `json:"public_group"`
	SortOrder         int     `json:"sort_order"`
}

func componentsPath(pageID string) string {
	return statusPagesPath + "/" + url.PathEscape(pageID) + "/components"
}

// AddComponent curates a monitor onto a page. The API returns 204 with no body;
// the caller reads the resulting component back via ListComponents.
func (c *Client) AddComponent(ctx context.Context, pageID string, in NewStatusPageComponent) error {
	return c.do(ctx, http.MethodPost, componentsPath(pageID), in, nil)
}

func (c *Client) ListComponents(ctx context.Context, pageID string) ([]StatusPageComponent, error) {
	var out []StatusPageComponent
	if err := c.do(ctx, http.MethodGet, componentsPath(pageID), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdateComponent(ctx context.Context, pageID, targetID string, in StatusPageComponentUpdate) error {
	return c.do(ctx, http.MethodPatch, componentsPath(pageID)+"/"+url.PathEscape(targetID), in, nil)
}

func (c *Client) RemoveComponent(ctx context.Context, pageID, targetID string) error {
	return c.do(ctx, http.MethodDelete, componentsPath(pageID)+"/"+url.PathEscape(targetID), nil, nil)
}
