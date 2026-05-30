package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// channelModel is the tfsdk view of an uptimepage_notification_channel.
type channelModel struct {
	ID      types.String       `tfsdk:"id"`
	Name    types.String       `tfsdk:"name"`
	Kind    types.String       `tfsdk:"kind"`
	Enabled types.Bool         `tfsdk:"enabled"`
	Config  channelConfigModel `tfsdk:"config"`
}

// channelConfigModel is the discriminated config block.
type channelConfigModel struct {
	Type     types.String         `tfsdk:"type"`
	Webhook  *webhookConfigModel  `tfsdk:"webhook"`
	Slack    *slackConfigModel    `tfsdk:"slack"`
	Telegram *telegramConfigModel `tfsdk:"telegram"`
}

type webhookConfigModel struct {
	URL     types.String `tfsdk:"url"`
	Headers types.Map    `tfsdk:"headers"`
}

type slackConfigModel struct {
	WebhookURL types.String `tfsdk:"webhook_url"`
}

type telegramConfigModel struct {
	BotToken types.String `tfsdk:"bot_token"`
	ChatID   types.String `tfsdk:"chat_id"`
}

func (m channelModel) toNew(ctx context.Context) (client.NewNotificationChannel, diag.Diagnostics) {
	cfg, diags := m.Config.toWire(ctx)
	return client.NewNotificationChannel{
		Name:    m.Name.ValueString(),
		Config:  cfg,
		Enabled: m.Enabled.ValueBool(),
	}, diags
}

func (m channelModel) toUpdate(ctx context.Context) (client.ChannelUpdate, diag.Diagnostics) {
	cfg, diags := m.Config.toWire(ctx)
	return client.ChannelUpdate{
		Name:    m.Name.ValueString(),
		Config:  cfg,
		Enabled: m.Enabled.ValueBool(),
	}, diags
}

func (c channelConfigModel) toWire(ctx context.Context) (client.ChannelConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	kind := c.Type.ValueString()
	out := client.ChannelConfig{Type: kind}

	switch kind {
	case client.ChannelTypeWebhook:
		if c.Webhook == nil {
			return out, missingBlock(kind)
		}
		out.Webhook = &client.WebhookConfig{
			URL:     c.Webhook.URL.ValueString(),
			Headers: mapToStrings(ctx, c.Webhook.Headers, &diags),
		}
	case client.ChannelTypeSlack:
		if c.Slack == nil {
			return out, missingBlock(kind)
		}
		out.Slack = &client.SlackConfig{WebhookURL: c.Slack.WebhookURL.ValueString()}
	case client.ChannelTypeTelegram:
		if c.Telegram == nil {
			return out, missingBlock(kind)
		}
		out.Telegram = &client.TelegramConfig{
			BotToken: c.Telegram.BotToken.ValueString(),
			ChatID:   c.Telegram.ChatID.ValueString(),
		}
	default:
		diags.AddError("Invalid config", fmt.Sprintf("unsupported channel type %q", kind))
	}
	return out, diags
}

// channelToModel maps a read channel into the model. prior preserves the
// write-only secrets the API returns redacted (webhook url/headers, slack
// webhook_url, telegram bot_token).
func channelToModel(ctx context.Context, prior channelModel, ch *client.NotificationChannel) (channelModel, diag.Diagnostics) {
	cfg, diags := configToModel(ctx, prior.Config, ch.Config)
	return channelModel{
		ID:      types.StringValue(ch.ID),
		Name:    types.StringValue(ch.Name),
		Kind:    types.StringValue(ch.Kind),
		Enabled: types.BoolValue(ch.Enabled),
		Config:  cfg,
	}, diags
}

func configToModel(ctx context.Context, prior channelConfigModel, cfg client.ChannelConfig) (channelConfigModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := channelConfigModel{Type: types.StringValue(cfg.Type)}

	switch {
	case cfg.Webhook != nil:
		priorURL, priorHeaders := types.StringNull(), types.MapNull(types.StringType)
		if prior.Webhook != nil {
			priorURL, priorHeaders = prior.Webhook.URL, prior.Webhook.Headers
		}
		out.Webhook = &webhookConfigModel{
			URL:     keepSecret(priorURL, &cfg.Webhook.URL),
			Headers: keepHeaders(ctx, priorHeaders, cfg.Webhook.Headers, &diags),
		}
	case cfg.Slack != nil:
		priorURL := types.StringNull()
		if prior.Slack != nil {
			priorURL = prior.Slack.WebhookURL
		}
		out.Slack = &slackConfigModel{WebhookURL: keepSecret(priorURL, &cfg.Slack.WebhookURL)}
	case cfg.Telegram != nil:
		priorToken := types.StringNull()
		if prior.Telegram != nil {
			priorToken = prior.Telegram.BotToken
		}
		out.Telegram = &telegramConfigModel{
			BotToken: keepSecret(priorToken, &cfg.Telegram.BotToken),
			ChatID:   types.StringValue(cfg.Telegram.ChatID),
		}
	default:
		diags.AddError("Unsupported channel type", fmt.Sprintf("channel type %q has no config", cfg.Type))
	}
	return out, diags
}

// keepHeaders preserves prior header state when the API redacts the values
// (every value comes back as the sentinel); otherwise it reflects the response.
func keepHeaders(ctx context.Context, prior types.Map, got map[string]string, diags *diag.Diagnostics) types.Map {
	for _, v := range got {
		if v == redactedSentinel {
			return prior
		}
	}
	if len(got) == 0 {
		return types.MapNull(types.StringType)
	}
	m, d := types.MapValueFrom(ctx, types.StringType, got)
	diags.Append(d...)
	return m
}
