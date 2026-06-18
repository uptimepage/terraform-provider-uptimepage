package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const channelsPath = "/api/v1/notification-channels"

const (
	ChannelTypeWebhook    = "webhook"
	ChannelTypeSlack      = "slack"
	ChannelTypeTelegram   = "telegram"
	ChannelTypeDiscord    = "discord"
	ChannelTypeMsTeams    = "msteams"
	ChannelTypeGoogleChat = "google_chat"
	ChannelTypeEmail      = "email"
	ChannelTypePagerDuty  = "pagerduty"
	ChannelTypeNtfy       = "ntfy"
	ChannelTypePushover   = "pushover"
	ChannelTypeWhatsApp   = "whatsapp"
	ChannelTypeSMS        = "sms"

	// Created only by the dashboard's one-tap Telegram linking; the API
	// rejects it in request bodies, so the provider cannot manage it.
	channelTypeTelegramApp = "telegram_app"
)

// NotificationChannel is the read shape. Kind is derived server-side from the
// config type. Secret-bearing config fields come back as "***".
type NotificationChannel struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Kind    string        `json:"kind"`
	Config  ChannelConfig `json:"config"`
	Enabled bool          `json:"enabled"`
	// Set once an email channel's address confirms its verification link;
	// absent for every other kind and for unverified addresses.
	VerifiedAt string `json:"verified_at,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// NewNotificationChannel is the POST body (kind is derived from config).
type NewNotificationChannel struct {
	Name    string        `json:"name"`
	Config  ChannelConfig `json:"config"`
	Enabled bool          `json:"enabled"`
}

// ChannelUpdate is the PATCH body. A nil Config is omitted: the server
// treats any config in the body as a full replacement (secrets rewritten,
// email verification reset), so callers send it only when it changed.
type ChannelUpdate struct {
	Name    string         `json:"name"`
	Config  *ChannelConfig `json:"config,omitempty"`
	Enabled bool           `json:"enabled"`
}

// ChannelConfig is the internally-tagged transport config (discriminator
// "type"). Exactly one variant pointer is set.
type ChannelConfig struct {
	Type       string            `json:"-"`
	Webhook    *WebhookConfig    `json:"-"`
	Slack      *SlackConfig      `json:"-"`
	Telegram   *TelegramConfig   `json:"-"`
	Discord    *DiscordConfig    `json:"-"`
	MsTeams    *MsTeamsConfig    `json:"-"`
	GoogleChat *GoogleChatConfig `json:"-"`
	Email      *EmailConfig      `json:"-"`
	PagerDuty  *PagerDutyConfig  `json:"-"`
	Ntfy       *NtfyConfig       `json:"-"`
	Pushover   *PushoverConfig   `json:"-"`
	WhatsApp   *WhatsAppConfig   `json:"-"`
	SMS        *SMSConfig        `json:"-"`
}

// WebhookConfig: url and header values are redacted on read.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SlackConfig: webhook_url is redacted on read.
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
}

// TelegramConfig: bot_token is redacted on read; chat_id is not.
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// DiscordConfig: webhook_url is redacted on read.
type DiscordConfig struct {
	WebhookURL string `json:"webhook_url"`
}

// MsTeamsConfig: webhook_url is redacted on read.
type MsTeamsConfig struct {
	WebhookURL string `json:"webhook_url"`
}

// GoogleChatConfig: webhook_url is redacted on read.
type GoogleChatConfig struct {
	WebhookURL string `json:"webhook_url"`
}

// EmailConfig: the recipient address; not redacted on read. Delivery starts
// only after the address confirms its verification mail (see
// NotificationChannel.VerifiedAt).
type EmailConfig struct {
	To string `json:"to"`
}

// PagerDutyConfig: routing_key (Events API v2 integration key) is redacted on read.
type PagerDutyConfig struct {
	RoutingKey string `json:"routing_key"`
}

// NtfyConfig: access_token is redacted on read. server_url defaults to
// https://ntfy.sh server-side when omitted; topic is required.
type NtfyConfig struct {
	ServerURL   string `json:"server_url,omitempty"`
	Topic       string `json:"topic"`
	AccessToken string `json:"access_token,omitempty"`
}

// PushoverConfig: token and user are both redacted on read. device is optional;
// emergency defaults false server-side.
type PushoverConfig struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Device    string `json:"device,omitempty"`
	Emergency bool   `json:"emergency"`
}

// WhatsAppConfig (Business Cloud API): access_token is redacted on read.
// language_code is optional (defaults to en at send time).
type WhatsAppConfig struct {
	AccessToken   string `json:"access_token"`
	PhoneNumberID string `json:"phone_number_id"`
	To            string `json:"to"`
	TemplateName  string `json:"template_name"`
	LanguageCode  string `json:"language_code,omitempty"`
}

// SMSConfig is the bring-your-own SMS gateway config. provider selects the
// gateway and only that gateway's fields are sent. The gateway secret
// (auth_token / api_key / api_secret / api_token, depending on provider) is
// redacted on read; account identifiers and routing are not.
type SMSConfig struct {
	Provider           string `json:"provider"`
	To                 string `json:"to"`
	From               string `json:"from"`
	AccountSID         string `json:"account_sid,omitempty"`
	AuthToken          string `json:"auth_token,omitempty"`
	APIKey             string `json:"api_key,omitempty"`
	APISecret          string `json:"api_secret,omitempty"`
	MessagingProfileID string `json:"messaging_profile_id,omitempty"`
	AuthID             string `json:"auth_id,omitempty"`
	ServicePlanID      string `json:"service_plan_id,omitempty"`
	APIToken           string `json:"api_token,omitempty"`
	Region             string `json:"region,omitempty"`
}

func (c ChannelConfig) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case ChannelTypeWebhook:
		if c.Webhook == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			WebhookConfig
		}{c.Type, *c.Webhook})
	case ChannelTypeSlack:
		if c.Slack == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			SlackConfig
		}{c.Type, *c.Slack})
	case ChannelTypeTelegram:
		if c.Telegram == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			TelegramConfig
		}{c.Type, *c.Telegram})
	case ChannelTypeDiscord:
		if c.Discord == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			DiscordConfig
		}{c.Type, *c.Discord})
	case ChannelTypeMsTeams:
		if c.MsTeams == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			MsTeamsConfig
		}{c.Type, *c.MsTeams})
	case ChannelTypeGoogleChat:
		if c.GoogleChat == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			GoogleChatConfig
		}{c.Type, *c.GoogleChat})
	case ChannelTypeEmail:
		if c.Email == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			EmailConfig
		}{c.Type, *c.Email})
	case ChannelTypePagerDuty:
		if c.PagerDuty == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			PagerDutyConfig
		}{c.Type, *c.PagerDuty})
	case ChannelTypeNtfy:
		if c.Ntfy == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			NtfyConfig
		}{c.Type, *c.Ntfy})
	case ChannelTypePushover:
		if c.Pushover == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			PushoverConfig
		}{c.Type, *c.Pushover})
	case ChannelTypeWhatsApp:
		if c.WhatsApp == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			WhatsAppConfig
		}{c.Type, *c.WhatsApp})
	case ChannelTypeSMS:
		if c.SMS == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			SMSConfig
		}{c.Type, *c.SMS})
	case "":
		return nil, fmt.Errorf("channel config has no type")
	default:
		return nil, fmt.Errorf("unsupported channel type %q", c.Type)
	}
}

func (c *ChannelConfig) UnmarshalJSON(data []byte) error {
	*c = ChannelConfig{}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	c.Type = probe.Type
	switch probe.Type {
	case ChannelTypeWebhook:
		c.Webhook = new(WebhookConfig)
		return json.Unmarshal(data, c.Webhook)
	case ChannelTypeSlack:
		c.Slack = new(SlackConfig)
		return json.Unmarshal(data, c.Slack)
	case ChannelTypeTelegram:
		c.Telegram = new(TelegramConfig)
		return json.Unmarshal(data, c.Telegram)
	case ChannelTypeDiscord:
		c.Discord = new(DiscordConfig)
		return json.Unmarshal(data, c.Discord)
	case ChannelTypeMsTeams:
		c.MsTeams = new(MsTeamsConfig)
		return json.Unmarshal(data, c.MsTeams)
	case ChannelTypeGoogleChat:
		c.GoogleChat = new(GoogleChatConfig)
		return json.Unmarshal(data, c.GoogleChat)
	case ChannelTypeEmail:
		c.Email = new(EmailConfig)
		return json.Unmarshal(data, c.Email)
	case ChannelTypePagerDuty:
		c.PagerDuty = new(PagerDutyConfig)
		return json.Unmarshal(data, c.PagerDuty)
	case ChannelTypeNtfy:
		c.Ntfy = new(NtfyConfig)
		return json.Unmarshal(data, c.Ntfy)
	case ChannelTypePushover:
		c.Pushover = new(PushoverConfig)
		return json.Unmarshal(data, c.Pushover)
	case ChannelTypeWhatsApp:
		c.WhatsApp = new(WhatsAppConfig)
		return json.Unmarshal(data, c.WhatsApp)
	case ChannelTypeSMS:
		c.SMS = new(SMSConfig)
		return json.Unmarshal(data, c.SMS)
	case channelTypeTelegramApp:
		return fmt.Errorf("channel type %q is linked through the dashboard's Telegram bot and cannot be managed by Terraform", probe.Type)
	default:
		return fmt.Errorf("unsupported channel type %q", probe.Type)
	}
}

func (c *Client) CreateChannel(ctx context.Context, in NewNotificationChannel) (*NotificationChannel, error) {
	var out NotificationChannel
	if err := c.do(ctx, http.MethodPost, channelsPath, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetChannel(ctx context.Context, id string) (*NotificationChannel, error) {
	var out NotificationChannel
	if err := c.do(ctx, http.MethodGet, channelsPath+"/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateChannel(ctx context.Context, id string, in ChannelUpdate) (*NotificationChannel, error) {
	var out NotificationChannel
	if err := c.do(ctx, http.MethodPatch, channelsPath+"/"+url.PathEscape(id), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, channelsPath+"/"+url.PathEscape(id), nil, nil)
}
