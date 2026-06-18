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

	t.Run("discord", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeDiscord), Discord: &discordConfigModel{WebhookURL: types.StringValue("https://real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeDiscord, Discord: &client.DiscordConfig{WebhookURL: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Discord.WebhookURL.ValueString() != "https://real" {
			t.Errorf("webhook_url not preserved: %q", got.Discord.WebhookURL.ValueString())
		}
	})

	t.Run("msteams", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeMsTeams), MsTeams: &msteamsConfigModel{WebhookURL: types.StringValue("https://real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeMsTeams, MsTeams: &client.MsTeamsConfig{WebhookURL: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.MsTeams.WebhookURL.ValueString() != "https://real" {
			t.Errorf("webhook_url not preserved: %q", got.MsTeams.WebhookURL.ValueString())
		}
	})

	t.Run("google_chat", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeGoogleChat), GoogleChat: &googleChatConfigModel{WebhookURL: types.StringValue("https://real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeGoogleChat, GoogleChat: &client.GoogleChatConfig{WebhookURL: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.GoogleChat.WebhookURL.ValueString() != "https://real" {
			t.Errorf("webhook_url not preserved: %q", got.GoogleChat.WebhookURL.ValueString())
		}
	})

	t.Run("email_not_redacted", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeEmail), Email: &emailConfigModel{To: types.StringValue("old@example.com")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeEmail, Email: &client.EmailConfig{To: "oncall@example.com"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Email.To.ValueString() != "oncall@example.com" {
			t.Errorf("to (non-secret) should reflect API: %q", got.Email.To.ValueString())
		}
	})

	t.Run("sms_keeps_redacted_secret_and_nulls_absent_fields", func(t *testing.T) {
		prior := channelConfigModel{
			Type: types.StringValue(client.ChannelTypeSMS),
			SMS:  &smsConfigModel{AuthToken: types.StringValue("real-token")},
		}
		// Twilio read: auth_token redacted, account_sid visible, every other
		// gateway's field absent from the response.
		cfg := client.ChannelConfig{Type: client.ChannelTypeSMS, SMS: &client.SMSConfig{
			Provider: "twilio", To: "+15551234567", From: "+15557654321",
			AccountSID: "AC0123456789ABCDEF0123456789ABCDEF", AuthToken: redactedSentinel,
		}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.SMS.AuthToken.ValueString() != "real-token" {
			t.Errorf("auth_token not preserved: %q", got.SMS.AuthToken.ValueString())
		}
		if got.SMS.AccountSID.ValueString() != "AC0123456789ABCDEF0123456789ABCDEF" {
			t.Errorf("account_sid (non-secret) should reflect API: %q", got.SMS.AccountSID.ValueString())
		}
		// Fields another gateway would use must read null, not "", so they
		// don't show a perpetual diff.
		for name, v := range map[string]types.String{
			"api_key": got.SMS.APIKey, "api_secret": got.SMS.APISecret,
			"auth_id": got.SMS.AuthID, "service_plan_id": got.SMS.ServicePlanID,
			"api_token": got.SMS.APIToken, "region": got.SMS.Region,
			"messaging_profile_id": got.SMS.MessagingProfileID,
		} {
			if !v.IsNull() {
				t.Errorf("absent field %q should be null, got %q", name, v.ValueString())
			}
		}
	})
}

func TestChannelToModel_VerifiedAt(t *testing.T) {
	ctx := context.Background()
	prior := channelModel{Config: channelConfigModel{
		Type:  types.StringValue(client.ChannelTypeEmail),
		Email: &emailConfigModel{To: types.StringValue("oncall@example.com")},
	}}

	unverified := &client.NotificationChannel{
		ID: "id-1", Name: "Mail", Kind: client.ChannelTypeEmail, Enabled: true,
		Config: client.ChannelConfig{Type: client.ChannelTypeEmail, Email: &client.EmailConfig{To: "oncall@example.com"}},
	}
	got, d := channelToModel(ctx, prior, unverified)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if !got.VerifiedAt.IsNull() {
		t.Errorf("verified_at should be null before verification, got %q", got.VerifiedAt.ValueString())
	}

	unverified.VerifiedAt = "2026-06-12T00:00:00Z"
	got, d = channelToModel(ctx, prior, unverified)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.VerifiedAt.ValueString() != "2026-06-12T00:00:00Z" {
		t.Errorf("verified_at = %q", got.VerifiedAt.ValueString())
	}
}

// A rename/toggle must not carry config: the server treats config in the
// PATCH as a replacement and would reset an email channel's verification.
func TestChannelModel_ToUpdateOmitsUnchangedConfig(t *testing.T) {
	ctx := context.Background()
	emailCfg := channelConfigModel{
		Type:  types.StringValue(client.ChannelTypeEmail),
		Email: &emailConfigModel{To: types.StringValue("oncall@example.com")},
	}
	prior := channelModel{Name: types.StringValue("Mail"), Config: emailCfg}

	renamed := channelModel{Name: types.StringValue("Mail v2"), Config: emailCfg}
	up, d := renamed.toUpdate(ctx, prior)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if up.Config != nil {
		t.Errorf("unchanged config must be omitted from the PATCH, got %+v", up.Config)
	}
	if up.Name != "Mail v2" {
		t.Errorf("name = %q", up.Name)
	}

	retargeted := channelModel{Name: types.StringValue("Mail"), Config: channelConfigModel{
		Type:  types.StringValue(client.ChannelTypeEmail),
		Email: &emailConfigModel{To: types.StringValue("other@example.com")},
	}}
	up, d = retargeted.toUpdate(ctx, prior)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if up.Config == nil || up.Config.Email == nil || up.Config.Email.To != "other@example.com" {
		t.Errorf("changed config must be sent, got %+v", up.Config)
	}
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
