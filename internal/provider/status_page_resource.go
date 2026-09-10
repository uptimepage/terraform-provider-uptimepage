package provider

import (
	"context"
	"regexp"

	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type statusPageAPI interface {
	CreateStatusPage(ctx context.Context, in client.NewStatusPage) (*client.StatusPage, error)
	GetStatusPage(ctx context.Context, id string) (*client.StatusPage, error)
	UpdateStatusPage(ctx context.Context, id string, in client.StatusPageUpdate) (*client.StatusPage, error)
	DeleteStatusPage(ctx context.Context, id string) error
}

var (
	_ resource.Resource                = (*statusPageResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageResource)(nil)
)

type statusPageResource struct {
	api statusPageAPI
}

func newStatusPageResource() resource.Resource {
	return &statusPageResource{}
}

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (r *statusPageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

// Mirrors the API's own predicate (scheme + host + 200 characters) so a bad
// URL fails at plan rather than mid-apply. RegexMatches alone would pass a
// host-less `https://`, and LengthAtMost counts bytes where the API counts
// characters.
type websiteURLValidator struct{}

func (websiteURLValidator) Description(context.Context) string {
	return "must be an http:// or https:// URL with a host, at most 200 characters"
}

func (v websiteURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v websiteURLValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	if utf8.RuneCountInString(raw) <= 200 && websiteURL.MatchString(raw) {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid Attribute Value", v.Description(ctx))
}

var websiteURL = regexp.MustCompile(`(?i)^https?://[^/?#\s]+`)

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A public status page: its own subdomain slug, branding, and a " +
			"curated set of monitors (added with uptimepage_status_page_component).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Status page id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				Required: true,
				Description: "Globally-unique slug that routes the public page's subdomain. " +
					"Lowercased by the API.",
			},
			"name": schema.StringAttribute{Required: true, Description: "Operator-facing page name."},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether the page is published (publicly reachable).",
			},
			"display_name": schema.StringAttribute{
				Optional:    true,
				Description: "Public header name. Falls back to the org name when unset.",
			},
			"about": schema.StringAttribute{
				Optional:    true,
				Description: "Public 'about' blurb shown on the page.",
			},
			"brand_color": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Accent color as a 6-digit hex like `#3b82f6`. Defaults to the configured brand color when unset.",
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`^#[0-9a-fA-F]{6}$`),
					"must be a 6-digit hex color like #3b82f6",
				)},
			},
			"style": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Visual theme.",
				Validators:  []validator.String{stringvalidator.OneOf(client.StatusPageStyles...)},
			},
			"show_powered_by": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Pin the 'powered by' footer on or off. Omit to inherit the deployment default. " +
					"Honoured only on plans that sell white-label; elsewhere the badge renders whatever this says.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"hide_from_search": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Serve the page, its incident pages and its feed with `noindex`. The URL keeps working for anyone who has it. " +
					"Omit to keep whatever the page already has, so a page hidden from the console is not republished by an apply.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"website_url": schema.StringAttribute{
				Optional: true,
				Description: "Your own site. The page header links here, so a reader who arrived from it can get back. " +
					"`http(s)` only, at most 200 characters.",
				Validators: []validator.String{websiteURLValidator{}},
			},
			"logo_url": schema.StringAttribute{
				Computed:    true,
				Description: "Versioned public logo URL, or null when no logo. Upload via the UI.",
			},
			"status_url": schema.StringAttribute{
				Computed:      true,
				Description:   "Live public URL of the page, or null when no public surface is mounted.",
				PlanModifiers: []planmodifier.String{keepWhileSlugUnchanged()},
			},
		},
	}
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.api.CreateStatusPage(ctx, plan.toNew())
	if err != nil {
		resp.Diagnostics.AddError("Create status page failed", err.Error())
		return
	}
	final, err := r.api.UpdateStatusPage(ctx, created.ID, plan.toUpdate())
	if err != nil {
		// The page exists but branding didn't apply. Persist what we have so a
		// re-apply reconciles via Update instead of re-creating into a slug clash.
		resp.Diagnostics.Append(resp.State.Set(ctx, statusPageToModel(plan, created))...)
		resp.Diagnostics.AddError("Apply status page branding failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageToModel(plan, final))...)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.api.GetStatusPage(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read status page failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageToModel(state, got))...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.api.UpdateStatusPage(ctx, plan.ID.ValueString(), plan.toUpdate())
	if err != nil {
		resp.Diagnostics.AddError("Update status page failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, statusPageToModel(plan, updated))...)
}

func (r *statusPageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteStatusPage(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete status page failed", err.Error())
	}
}

func (r *statusPageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
