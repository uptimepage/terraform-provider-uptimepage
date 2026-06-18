package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type channelAPI interface {
	CreateChannel(ctx context.Context, in client.NewNotificationChannel) (*client.NotificationChannel, error)
	GetChannel(ctx context.Context, id string) (*client.NotificationChannel, error)
	UpdateChannel(ctx context.Context, id string, in client.ChannelUpdate) (*client.NotificationChannel, error)
	DeleteChannel(ctx context.Context, id string) error
}

var (
	_ resource.Resource                   = (*channelResource)(nil)
	_ resource.ResourceWithConfigure      = (*channelResource)(nil)
	_ resource.ResourceWithImportState    = (*channelResource)(nil)
	_ resource.ResourceWithValidateConfig = (*channelResource)(nil)
)

type channelResource struct {
	api channelAPI
}

func newChannelResource() resource.Resource {
	return &channelResource{}
}

func (r *channelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_channel"
}

func (r *channelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *channelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A notification channel (webhook, Slack, Telegram, Discord, Microsoft Teams, Google Chat, email, PagerDuty, ntfy, Pushover, WhatsApp, or SMS).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Channel id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Channel name."},
			"kind": schema.StringAttribute{
				Computed:    true,
				Description: "Channel kind, derived from config.type. Recomputed on apply.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the channel is active.",
			},
			"verified_at": schema.StringAttribute{
				Computed: true,
				Description: "When the email address confirmed its verification link (RFC 3339). " +
					"Null for non-email channels and until the recipient verifies; email channels " +
					"deliver only after verification.",
			},
			"config": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Transport config. Set `type` and the matching nested block.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:    true,
						Description: "Channel type: webhook, slack, telegram, discord, msteams, google_chat, email, pagerduty, ntfy, pushover, whatsapp, sms. The dashboard's one-tap telegram_app kind is not manageable here.",
						Validators: []validator.String{stringvalidator.OneOf(
							client.ChannelTypeWebhook, client.ChannelTypeSlack, client.ChannelTypeTelegram,
							client.ChannelTypeDiscord, client.ChannelTypeMsTeams, client.ChannelTypeGoogleChat,
							client.ChannelTypeEmail, client.ChannelTypePagerDuty, client.ChannelTypeNtfy,
							client.ChannelTypePushover, client.ChannelTypeWhatsApp, client.ChannelTypeSMS)},
					},
					"webhook": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Generic webhook (when type = webhook).",
						Attributes: map[string]schema.Attribute{
							"url": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Webhook URL. Write-only: returned redacted by the API.",
							},
							"headers": schema.MapAttribute{
								Optional:    true,
								Sensitive:   true,
								ElementType: types.StringType,
								Description: "Extra request headers. Write-only values.",
							},
						},
					},
					"slack": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Slack incoming webhook (when type = slack).",
						Attributes: map[string]schema.Attribute{
							"webhook_url": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Slack webhook URL. Write-only.",
							},
						},
					},
					"telegram": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Telegram bot (when type = telegram).",
						Attributes: map[string]schema.Attribute{
							"bot_token": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Bot token. Write-only.",
							},
							"chat_id": schema.StringAttribute{Required: true, Description: "Target chat id."},
						},
					},
					"discord": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Discord channel webhook (when type = discord).",
						Attributes: map[string]schema.Attribute{
							"webhook_url": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Discord webhook URL. Write-only.",
							},
						},
					},
					"msteams": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Microsoft Teams incoming webhook (when type = msteams).",
						Attributes: map[string]schema.Attribute{
							"webhook_url": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Teams workflow/webhook URL. Write-only.",
							},
						},
					},
					"google_chat": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Google Chat space webhook (when type = google_chat).",
						Attributes: map[string]schema.Attribute{
							"webhook_url": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "Google Chat webhook URL. Write-only.",
							},
						},
					},
					"email": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Email recipient (when type = email). Delivery starts only after the address confirms the verification mail; track it via verified_at.",
						Attributes: map[string]schema.Attribute{
							"to": schema.StringAttribute{
								Required:    true,
								Description: "Recipient address (lowercase).",
								Validators: []validator.String{
									stringvalidator.RegexMatches(
										regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`),
										"must be an email address"),
									stringvalidator.RegexMatches(
										regexp.MustCompile(`^[^A-Z]*$`),
										"must be lowercase"),
								},
							},
						},
					},
					"pagerduty": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "PagerDuty Events API v2 (when type = pagerduty).",
						Attributes: map[string]schema.Attribute{
							"routing_key": schema.StringAttribute{
								Required: true, Sensitive: true,
								Description: "Events API v2 integration (routing) key. Write-only.",
							},
						},
					},
					"ntfy": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "ntfy topic push (when type = ntfy).",
						Attributes: map[string]schema.Attribute{
							"server_url": schema.StringAttribute{
								Optional: true,
								// Server defaults an omitted server_url to https://ntfy.sh;
								// Computed adopts that read-back so it is not a perpetual diff.
								Computed:      true,
								Description:   "ntfy server root. Defaults to https://ntfy.sh.",
								PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
							},
							"topic": schema.StringAttribute{Required: true, Description: "Topic to publish to."},
							"access_token": schema.StringAttribute{
								Optional: true, Sensitive: true,
								Description: "Bearer token for protected topics. Write-only.",
							},
						},
					},
					"pushover": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Pushover (when type = pushover).",
						Attributes: map[string]schema.Attribute{
							"token": schema.StringAttribute{
								Required: true, Sensitive: true, Description: "Application API token. Write-only.",
							},
							"user": schema.StringAttribute{
								Required: true, Sensitive: true, Description: "User or group key. Write-only.",
							},
							"device": schema.StringAttribute{
								Optional: true, Description: "Target device name; empty delivers to all devices.",
							},
							"emergency": schema.BoolAttribute{
								Optional:    true,
								Computed:    true,
								Default:     booldefault.StaticBool(false),
								Description: "Send high-urgency alerts at emergency priority (repeat until acknowledged).",
							},
						},
					},
					"whatsapp": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "WhatsApp Business Cloud API (when type = whatsapp).",
						Attributes: map[string]schema.Attribute{
							"access_token": schema.StringAttribute{
								Required: true, Sensitive: true, Description: "Cloud API access token. Write-only.",
							},
							"phone_number_id": schema.StringAttribute{
								Required:    true,
								Description: "Business phone number id (numeric).",
								Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^\d+$`), "must be numeric")},
							},
							"to": schema.StringAttribute{
								Required: true, Description: "Recipient phone number in international format.",
							},
							"template_name": schema.StringAttribute{
								Required:    true,
								Description: "Approved template name (lowercase letters, digits, underscore).",
								Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9_]+$`), "must be lowercase letters, digits, or underscore")},
							},
							"language_code": schema.StringAttribute{
								Optional: true, Description: "Template language (e.g. en, en_US). Defaults to en.",
							},
						},
					},
					"sms": schema.SingleNestedAttribute{
						Optional: true,
						Description: "Bring-your-own SMS gateway (when type = sms). Set `provider` and that " +
							"gateway's credentials; unused fields are ignored.",
						Attributes: map[string]schema.Attribute{
							"provider": schema.StringAttribute{
								Required:    true,
								Description: "SMS gateway: twilio, telnyx, vonage, plivo, sinch.",
								Validators:  []validator.String{stringvalidator.OneOf("twilio", "telnyx", "vonage", "plivo", "sinch")},
							},
							"to": schema.StringAttribute{
								Required:    true,
								Description: "Recipient phone number in E.164 format (e.g. +15551234567).",
								Validators: []validator.String{stringvalidator.RegexMatches(
									regexp.MustCompile(`^\+[1-9]\d{7,14}$`), "must be E.164, e.g. +15551234567")},
							},
							"from": schema.StringAttribute{
								Required:    true,
								Description: "Sender: an E.164 number, alphanumeric sender id, or messaging-service id.",
							},
							"account_sid": schema.StringAttribute{
								Optional: true, Description: "Twilio Account SID (provider = twilio).",
							},
							"auth_token": schema.StringAttribute{
								Optional: true, Sensitive: true,
								Description: "Twilio/Plivo auth token (provider = twilio or plivo). Write-only.",
							},
							"api_key": schema.StringAttribute{
								Optional: true, Sensitive: true,
								Description: "Telnyx/Vonage API key (provider = telnyx or vonage). Write-only.",
							},
							"api_secret": schema.StringAttribute{
								Optional: true, Sensitive: true,
								Description: "Vonage API secret (provider = vonage). Write-only.",
							},
							"messaging_profile_id": schema.StringAttribute{
								Optional: true, Description: "Telnyx messaging profile id (provider = telnyx, optional).",
							},
							"auth_id": schema.StringAttribute{
								Optional: true, Description: "Plivo Auth ID (provider = plivo).",
							},
							"service_plan_id": schema.StringAttribute{
								Optional: true, Description: "Sinch service plan id (provider = sinch).",
							},
							"api_token": schema.StringAttribute{
								Optional: true, Sensitive: true,
								Description: "Sinch API token (provider = sinch). Write-only.",
							},
							"region": schema.StringAttribute{
								Optional: true,
								// Sinch defaults an omitted region to us server-side; Computed
								// adopts that read-back so an unset region is not a perpetual diff.
								Computed:      true,
								Description:   "Sinch cluster region (provider = sinch): us, eu, au, br, ca. Defaults to us.",
								Validators:    []validator.String{stringvalidator.OneOf("us", "eu", "au", "br", "ca")},
								PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
							},
						},
					},
				},
			},
		},
	}
}

func (r *channelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg channelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Config.Type.IsUnknown() || cfg.Config.Type.IsNull() {
		return
	}
	validateDiscriminatedBlock(path.Root("config"), cfg.Config.Type.ValueString(), map[string]bool{
		client.ChannelTypeWebhook:    cfg.Config.Webhook != nil,
		client.ChannelTypeSlack:      cfg.Config.Slack != nil,
		client.ChannelTypeTelegram:   cfg.Config.Telegram != nil,
		client.ChannelTypeDiscord:    cfg.Config.Discord != nil,
		client.ChannelTypeMsTeams:    cfg.Config.MsTeams != nil,
		client.ChannelTypeGoogleChat: cfg.Config.GoogleChat != nil,
		client.ChannelTypeEmail:      cfg.Config.Email != nil,
		client.ChannelTypePagerDuty:  cfg.Config.PagerDuty != nil,
		client.ChannelTypeNtfy:       cfg.Config.Ntfy != nil,
		client.ChannelTypePushover:   cfg.Config.Pushover != nil,
		client.ChannelTypeWhatsApp:   cfg.Config.WhatsApp != nil,
		client.ChannelTypeSMS:        cfg.Config.SMS != nil,
	}, &resp.Diagnostics)
}

func (r *channelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan channelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, d := plan.toNew(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.api.CreateChannel(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create channel failed", err.Error())
		return
	}
	state, d := channelToModel(ctx, plan, created)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *channelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state channelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.api.GetChannel(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read channel failed", err.Error())
		return
	}
	next, d := channelToModel(ctx, state, got)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *channelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state channelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, d := plan.toUpdate(ctx, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.api.UpdateChannel(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update channel failed", err.Error())
		return
	}
	next, d := channelToModel(ctx, plan, updated)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *channelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state channelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteChannel(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete channel failed", err.Error())
	}
}

func (r *channelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
