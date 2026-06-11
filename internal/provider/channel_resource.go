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
		Description: "A notification channel (webhook, Slack, Telegram, Discord, Microsoft Teams, Google Chat, or email).",
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
						Description: "Channel type: webhook, slack, telegram, discord, msteams, google_chat, email. The dashboard's one-tap telegram_app kind is not manageable here.",
						Validators: []validator.String{stringvalidator.OneOf(
							client.ChannelTypeWebhook, client.ChannelTypeSlack, client.ChannelTypeTelegram,
							client.ChannelTypeDiscord, client.ChannelTypeMsTeams, client.ChannelTypeGoogleChat,
							client.ChannelTypeEmail)},
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
