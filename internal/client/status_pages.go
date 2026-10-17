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

// StatusPageLocales is the set of accepted public_locale values, mirrored from
// the API's CHECK list.
var StatusPageLocales = []string{"en", "de"}

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
	PublicShowPoweredBy  *bool   `json:"public_show_powered_by,omitempty"`
	ShowPoweredBy        bool    `json:"show_powered_by"`
	PublicHideFromSearch bool    `json:"public_hide_from_search"`
	PublicWebsiteURL     *string `json:"public_website_url,omitempty"`
	PublicLocale         *string `json:"public_locale,omitempty"`
	LogoURL              *string `json:"logo_url,omitempty"`
	StatusURL            *string `json:"status_url,omitempty"`
	// Nil until the page's access is first written.
	Access *PageAccess `json:"access,omitempty"`
}

// PageAccess is who may open a page, read and written whole. Protected makes
// it private; while it is false the server keeps the rest for next time.
type PageAccess struct {
	Protected bool `json:"protected"`
	// For email and password sign-ins alike. Nil keeps one until sign-out.
	SessionDays *int64             `json:"session_days"`
	Email       PageEmailAccess    `json:"email"`
	Password    PagePasswordAccess `json:"password"`
	Rules       []PageAccessRule   `json:"rules"`
}

// PageEmailAccess is the emailed-code way in.
type PageEmailAccess struct {
	Enabled         bool `json:"enabled"`
	PasskeysEnabled bool `json:"passkeys_enabled"`
	// Nil never asks a passkey viewer for a code again.
	RecheckDays *int64 `json:"recheck_days"`
}

// PagePasswordAccess is the shared page password way in. The server stores a
// hash and never returns the password; Value goes out only to set or change
// it, since every new value signs out everyone who used the old one.
type PagePasswordAccess struct {
	Enabled bool    `json:"enabled"`
	Value   *string `json:"value,omitempty"`
}

// PageAccessRule is one entry of the list: an address or a whole domain.
type PageAccessRule struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

const (
	AccessRuleAddress = "address"
	AccessRuleDomain  = "domain"
)

// PageAccessRecheckDays and PageAccessSessionDays mirror the API's choices.
var (
	PageAccessRecheckDays = []int64{30, 90, 365}
	PageAccessSessionDays = []int64{7, 30, 90}
)

// MaxPageAccessRules is the API's cap on one page's list,
// MaxPageAccessAddressLen its cap on one address, and MaxPagePasswordChars
// its cap on the page password.
const (
	MaxPageAccessRules      = 500
	MaxPageAccessAddressLen = 254
	MaxPagePasswordChars    = 128
)

// NewStatusPage is the POST body. Branding is set via a follow-up PATCH, so only
// identity, enabled and access travel here.
type NewStatusPage struct {
	Slug    string      `json:"slug"`
	Name    string      `json:"name"`
	Enabled bool        `json:"enabled"`
	Access  *PageAccess `json:"access,omitempty"`
}

// StatusPageUpdate is the PATCH body, sent as full desired state. Branding is a
// nested block that replaces the page's display branding wholesale. The
// language and access are partial fields beside it: nil leaves them as is.
type StatusPageUpdate struct {
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Enabled      bool           `json:"enabled"`
	PublicLocale *string        `json:"public_locale,omitempty"`
	Branding     StatusBranding `json:"branding"`
	Access       *PageAccess    `json:"access,omitempty"`
}

// StatusBranding is the page's display branding, sent whole on every PATCH. A
// nil pointer marshals to JSON null: display_name, about, brand_color and
// website_url clear, style falls back to the server default, and
// hide_from_search reads as false — null does not mean "leave as is".
// show_powered_by is accepted and ignored while the badge is pinned on.
type StatusBranding struct {
	PublicDisplayName    *string `json:"public_display_name"`
	PublicAbout          *string `json:"public_about"`
	PublicBrandColor     *string `json:"public_brand_color"`
	PublicStyle          *string `json:"public_style"`
	PublicShowPoweredBy  *bool   `json:"public_show_powered_by"`
	PublicHideFromSearch *bool   `json:"public_hide_from_search"`
	PublicWebsiteURL     *string `json:"public_website_url"`
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
