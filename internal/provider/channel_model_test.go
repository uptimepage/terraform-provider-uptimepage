package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func TestChannelConfig_RedactionSuppressed(t *testing.T) {
	ctx := context.Background()
	priorHeaders, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"X-Token": "real-secret"})

	t.Run("webhook", func(t *testing.T) {
		prior := channelConfigModel{
			Type:    types.StringValue(client.ChannelTypeWebhook),
			Webhook: &webhookConfigModel{URL: types.StringValue("https://real"), Headers: priorHeaders},
		}
		cfg := client.ChannelConfig{Type: client.ChannelTypeWebhook, Webhook: &client.WebhookConfig{
			URL: redactedSentinel, Headers: map[string]string{"X-Token": redactedSentinel},
		}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Webhook.URL.ValueString() != "https://real" {
			t.Errorf("url not preserved: %q", got.Webhook.URL.ValueString())
		}
		var h map[string]string
		got.Webhook.Headers.ElementsAs(ctx, &h, false)
		if h["X-Token"] != "real-secret" {
			t.Errorf("headers not preserved: %v", h)
		}
	})

	t.Run("slack", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeSlack), Slack: &slackConfigModel{WebhookURL: types.StringValue("https://real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeSlack, Slack: &client.SlackConfig{WebhookURL: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Slack.WebhookURL.ValueString() != "https://real" {
			t.Errorf("webhook_url not preserved: %q", got.Slack.WebhookURL.ValueString())
		}
	})

	t.Run("telegram", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeTelegram), Telegram: &telegramConfigModel{BotToken: types.StringValue("real-token")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeTelegram, Telegram: &client.TelegramConfig{BotToken: redactedSentinel, ChatID: "-100"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Telegram.BotToken.ValueString() != "real-token" {
			t.Errorf("bot_token not preserved: %q", got.Telegram.BotToken.ValueString())
		}
		if got.Telegram.ChatID.ValueString() != "-100" {
			t.Errorf("chat_id (non-secret) should reflect API: %q", got.Telegram.ChatID.ValueString())
		}
	})
}

func TestChannelConfig_ToWireMissingBlock(t *testing.T) {
	ctx := context.Background()
	c := channelConfigModel{Type: types.StringValue(client.ChannelTypeSlack)} // no slack block
	_, d := c.toWire(ctx)
	if !d.HasError() {
		t.Error("expected error when the block for the type is missing")
	}
}

func TestChannelConfig_ToWireWebhook(t *testing.T) {
	ctx := context.Background()
	headers, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"X-A": "1"})
	c := channelConfigModel{
		Type:    types.StringValue(client.ChannelTypeWebhook),
		Webhook: &webhookConfigModel{URL: types.StringValue("https://x"), Headers: headers},
	}
	out, d := c.toWire(ctx)
	if d.HasError() {
		t.Fatalf("toWire: %v", d)
	}
	if out.Webhook == nil || out.Webhook.URL != "https://x" || out.Webhook.Headers["X-A"] != "1" {
		t.Errorf("webhook wire wrong: %+v", out.Webhook)
	}
}
