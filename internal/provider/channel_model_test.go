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

	t.Run("pagerduty_routing_key_redacted", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypePagerDuty), PagerDuty: &pagerdutyConfigModel{RoutingKey: types.StringValue("real-key")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypePagerDuty, PagerDuty: &client.PagerDutyConfig{RoutingKey: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.PagerDuty.RoutingKey.ValueString() != "real-key" {
			t.Errorf("routing_key not preserved: %q", got.PagerDuty.RoutingKey.ValueString())
		}
	})

	t.Run("ntfy_token_redacted_serverurl_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeNtfy), Ntfy: &ntfyConfigModel{AccessToken: types.StringValue("real-tok")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeNtfy, Ntfy: &client.NtfyConfig{ServerURL: "https://ntfy.sh", Topic: "ops", AccessToken: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Ntfy.AccessToken.ValueString() != "real-tok" {
			t.Errorf("access_token not preserved: %q", got.Ntfy.AccessToken.ValueString())
		}
		if got.Ntfy.ServerURL.ValueString() != "https://ntfy.sh" || got.Ntfy.Topic.ValueString() != "ops" {
			t.Errorf("non-secret ntfy fields should reflect API: %+v", got.Ntfy)
		}
	})

	t.Run("gotify_token_redacted_serverurl_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeGotify), Gotify: &gotifyConfigModel{Token: types.StringValue("real-tok")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeGotify, Gotify: &client.GotifyConfig{ServerURL: "https://push.example.com/gotify", Token: redactedSentinel}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Gotify.Token.ValueString() != "real-tok" {
			t.Errorf("token not preserved: %q", got.Gotify.Token.ValueString())
		}
		if got.Gotify.ServerURL.ValueString() != "https://push.example.com/gotify" {
			t.Errorf("non-secret gotify fields should reflect API: %+v", got.Gotify)
		}
	})

	t.Run("slack_url_redacted_mention_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeSlack), Slack: &slackConfigModel{WebhookURL: types.StringValue("https://hooks.slack.com/services/real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeSlack, Slack: &client.SlackConfig{WebhookURL: redactedSentinel, Mention: "@here S01ABC2345"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Slack.WebhookURL.ValueString() != "https://hooks.slack.com/services/real" {
			t.Errorf("webhook_url not preserved: %q", got.Slack.WebhookURL.ValueString())
		}
		if got.Slack.Mention.ValueString() != "@here S01ABC2345" {
			t.Errorf("mention should reflect the API: %+v", got.Slack)
		}
	})

	// Slack ids are uppercase, so unlike Mattermost the read-back must not fold.
	t.Run("slack_mention_case_is_significant", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeSlack), Slack: &slackConfigModel{Mention: types.StringValue("U01ABC2345")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeSlack, Slack: &client.SlackConfig{WebhookURL: redactedSentinel, Mention: "u01abc2345"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Slack.Mention.ValueString() != "u01abc2345" {
			t.Errorf("a case change is real drift on Slack, got %q", got.Slack.Mention.ValueString())
		}
	})

	t.Run("discord_url_redacted_mention_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeDiscord), Discord: &discordConfigModel{WebhookURL: types.StringValue("https://discord.com/api/webhooks/1/real")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeDiscord, Discord: &client.DiscordConfig{WebhookURL: redactedSentinel, Mention: "&123456789012345678"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Discord.WebhookURL.ValueString() != "https://discord.com/api/webhooks/1/real" {
			t.Errorf("webhook_url not preserved: %q", got.Discord.WebhookURL.ValueString())
		}
		if got.Discord.Mention.ValueString() != "&123456789012345678" {
			t.Errorf("mention should reflect the API: %+v", got.Discord)
		}
	})

	t.Run("slack_absent_mention_is_null", func(t *testing.T) {
		cfg := client.ChannelConfig{Type: client.ChannelTypeSlack, Slack: &client.SlackConfig{WebhookURL: "https://hooks.slack.com/services/x"}}
		got, d := configToModel(ctx, channelConfigModel{}, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if !got.Slack.Mention.IsNull() {
			t.Errorf("absent mention should be null, got %q", got.Slack.Mention.ValueString())
		}
	})

	t.Run("mattermost_url_redacted_mention_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeMattermost), Mattermost: &mattermostConfigModel{WebhookURL: types.StringValue("https://mm.example.com/hooks/real-key")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeMattermost, Mattermost: &client.MattermostConfig{WebhookURL: redactedSentinel, Mention: "@here"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Mattermost.WebhookURL.ValueString() != "https://mm.example.com/hooks/real-key" {
			t.Errorf("webhook_url not preserved: %q", got.Mattermost.WebhookURL.ValueString())
		}
		if got.Mattermost.Mention.ValueString() != "@here" {
			t.Errorf("mention should reflect the API: %+v", got.Mattermost)
		}
	})

	t.Run("mattermost_mention_keeps_the_config_spelling_when_only_case_differs", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeMattermost), Mattermost: &mattermostConfigModel{
			WebhookURL: types.StringValue("https://mm.example.com/hooks/real-key"),
			Mention:    types.StringValue("@Here, OnCall-SRE"),
		}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeMattermost, Mattermost: &client.MattermostConfig{WebhookURL: redactedSentinel, Mention: "@here, oncall-sre"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Mattermost.Mention.ValueString() != "@Here, OnCall-SRE" {
			t.Errorf("case-only change should keep the config spelling, got %q", got.Mattermost.Mention.ValueString())
		}
	})

	t.Run("mattermost_mention_reflects_a_real_change", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeMattermost), Mattermost: &mattermostConfigModel{Mention: types.StringValue("@here")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeMattermost, Mattermost: &client.MattermostConfig{WebhookURL: redactedSentinel, Mention: "@channel"}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Mattermost.Mention.ValueString() != "@channel" {
			t.Errorf("real drift should surface, got %q", got.Mattermost.Mention.ValueString())
		}
	})

	t.Run("mattermost_absent_mention_is_null", func(t *testing.T) {
		cfg := client.ChannelConfig{Type: client.ChannelTypeMattermost, Mattermost: &client.MattermostConfig{WebhookURL: "https://mm.example.com/hooks/k"}}
		got, d := configToModel(ctx, channelConfigModel{}, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if !got.Mattermost.Mention.IsNull() {
			t.Errorf("absent mention should be null, got %q", got.Mattermost.Mention.ValueString())
		}
	})

	t.Run("pushover_both_keys_redacted_emergency_reflected", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypePushover), Pushover: &pushoverConfigModel{Token: types.StringValue("real-token"), User: types.StringValue("real-user")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypePushover, Pushover: &client.PushoverConfig{Token: redactedSentinel, User: redactedSentinel, Emergency: true}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.Pushover.Token.ValueString() != "real-token" || got.Pushover.User.ValueString() != "real-user" {
			t.Errorf("pushover secrets not preserved: %+v", got.Pushover)
		}
		if !got.Pushover.Emergency.ValueBool() {
			t.Errorf("emergency should reflect API true")
		}
	})

	t.Run("whatsapp_token_redacted_rest_visible", func(t *testing.T) {
		prior := channelConfigModel{Type: types.StringValue(client.ChannelTypeWhatsApp), WhatsApp: &whatsappConfigModel{AccessToken: types.StringValue("real-tok")}}
		cfg := client.ChannelConfig{Type: client.ChannelTypeWhatsApp, WhatsApp: &client.WhatsAppConfig{
			AccessToken: redactedSentinel, PhoneNumberID: "123", To: "15551234567", TemplateName: "uptime_alert",
		}}
		got, d := configToModel(ctx, prior, cfg)
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got.WhatsApp.AccessToken.ValueString() != "real-tok" {
			t.Errorf("access_token not preserved: %q", got.WhatsApp.AccessToken.ValueString())
		}
		if got.WhatsApp.PhoneNumberID.ValueString() != "123" || got.WhatsApp.To.ValueString() != "15551234567" {
			t.Errorf("non-secret whatsapp fields should reflect API: %+v", got.WhatsApp)
		}
		if !got.WhatsApp.LanguageCode.IsNull() {
			t.Errorf("absent language_code should be null, got %q", got.WhatsApp.LanguageCode.ValueString())
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
