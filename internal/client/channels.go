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
	ChannelTypeWebhook  = "webhook"
	ChannelTypeSlack    = "slack"
	ChannelTypeTelegram = "telegram"

	// Created only by the dashboard's one-tap Telegram linking; the API
	// rejects it in request bodies, so the provider cannot manage it.
	channelTypeTelegramApp = "telegram_app"
)

// NotificationChannel is the read shape. Kind is derived server-side from the
// config type. Secret-bearing config fields come back as "***".
type NotificationChannel struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Config    ChannelConfig `json:"config"`
	Enabled   bool          `json:"enabled"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
}

// NewNotificationChannel is the POST body (kind is derived from config).
type NewNotificationChannel struct {
	Name    string        `json:"name"`
	Config  ChannelConfig `json:"config"`
	Enabled bool          `json:"enabled"`
}

// ChannelUpdate is the PATCH body, sent as full desired state.
type ChannelUpdate struct {
	Name    string        `json:"name"`
	Config  ChannelConfig `json:"config"`
	Enabled bool          `json:"enabled"`
}

// ChannelConfig is the internally-tagged transport config (discriminator
// "type"). Exactly one variant pointer is set.
type ChannelConfig struct {
	Type     string          `json:"-"`
	Webhook  *WebhookConfig  `json:"-"`
	Slack    *SlackConfig    `json:"-"`
	Telegram *TelegramConfig `json:"-"`
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
