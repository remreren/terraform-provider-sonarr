package provider

import (
	"context"
	"strconv"

	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/devopsarr/terraform-provider-sonarr/internal/helpers"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const releaseProfileResourceName = "release_profile"

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &ReleaseProfileResource{}
	_ resource.ResourceWithImportState = &ReleaseProfileResource{}
)

func NewReleaseProfileResource() resource.Resource {
	return &ReleaseProfileResource{}
}

// ReleaseProfileResource defines the release profile implementation.
type ReleaseProfileResource struct {
	client *sonarr.APIClient
	auth   context.Context
}

// ReleaseProfile describes the release profile data model. ExcludedTags, AirDateGracePeriod,
// AirDateRestriction and AllowSeasonPackNotAired are Sonarr v5+.
type ReleaseProfile struct {
	Tags                    types.Set    `tfsdk:"tags"`
	Ignored                 types.Set    `tfsdk:"ignored"`
	Required                types.Set    `tfsdk:"required"`
	ExcludedTags            types.Set    `tfsdk:"excluded_tags"`
	Name                    types.String `tfsdk:"name"`
	ID                      types.Int64  `tfsdk:"id"`
	IndexerID               types.Int64  `tfsdk:"indexer_id"`
	AirDateGracePeriod      types.Int64  `tfsdk:"air_date_grace_period"`
	Enabled                 types.Bool   `tfsdk:"enabled"`
	AirDateRestriction      types.Bool   `tfsdk:"air_date_restriction"`
	AllowSeasonPackNotAired types.Bool   `tfsdk:"allow_season_pack_without_all_episodes_aired"`
}

func (p ReleaseProfile) getType() attr.Type {
	return types.ObjectType{}.WithAttributeTypes(
		map[string]attr.Type{
			"tags":                  types.SetType{}.WithElementType(types.Int64Type),
			"ignored":               types.SetType{}.WithElementType(types.StringType),
			"required":              types.SetType{}.WithElementType(types.StringType),
			"name":                  types.StringType,
			"id":                    types.Int64Type,
			"indexer_id":            types.Int64Type,
			"enabled":               types.BoolType,
			"excluded_tags":         types.SetType{}.WithElementType(types.Int64Type),
			"air_date_grace_period": types.Int64Type,
			"air_date_restriction":  types.BoolType,
			"allow_season_pack_without_all_episodes_aired": types.BoolType,
		})
}

func (r *ReleaseProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + releaseProfileResourceName
}

func (r *ReleaseProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "<!-- subcategory:Profiles -->\nRelease Profile resource.\nFor more information refer to [Release Profiles](https://wiki.servarr.com/sonarr/settings#release-profiles) documentation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "Release Profile ID.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Enabled.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Release profile name.",
				Optional:            true,
				Computed:            true,
			},
			"indexer_id": schema.Int64Attribute{
				MarkdownDescription: "Indexer ID. Default to all.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"required": schema.SetAttribute{
				MarkdownDescription: "Required terms. At least one of `required` and `ignored` must be set.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
			},
			"ignored": schema.SetAttribute{
				MarkdownDescription: "Ignored terms. At least one of `required` and `ignored` must be set.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "List of associated tags.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.Int64Type,
			},
			"excluded_tags": schema.SetAttribute{
				MarkdownDescription: "Tags of series this profile does not apply to. Sonarr v5+; not configured: kept as it is in Sonarr.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.Int64Type,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"air_date_restriction": schema.BoolAttribute{
				MarkdownDescription: "Only apply to releases of episodes that aired (within air_date_grace_period days). Sonarr v5+; not configured: kept as it is in Sonarr.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"air_date_grace_period": schema.Int64Attribute{
				MarkdownDescription: "Days after the air date (air_date_restriction). Sonarr v5+; not configured: kept as it is in Sonarr.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"allow_season_pack_without_all_episodes_aired": schema.BoolAttribute{
				MarkdownDescription: "Allow season packs before every episode aired. Sonarr v5+; not configured: kept as it is in Sonarr.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ReleaseProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if auth, client := resourceConfigure(ctx, req, resp); client != nil {
		r.client = client
		r.auth = auth
	}
}

func (r *ReleaseProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var profile *ReleaseProfile

	resp.Diagnostics.Append(req.Plan.Get(ctx, &profile)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Build Create resource
	request := profile.read(ctx, &resp.Diagnostics)

	// Create new ReleaseProfile
	response, _, err := r.client.ReleaseProfileAPI.CreateReleaseProfile(r.auth).ReleaseProfileResource(*request).Execute()
	if err != nil {
		resp.Diagnostics.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Create, releaseProfileResourceName, err))

		return
	}

	tflog.Trace(ctx, "created"+releaseProfileResourceName+": "+strconv.Itoa(int(response.GetId())))
	// Generate resource state struct
	profile.write(ctx, response, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &profile)...)
}

func (r *ReleaseProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var profile *ReleaseProfile

	resp.Diagnostics.Append(req.State.Get(ctx, &profile)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get releaseprofile current value
	response, _, err := r.client.ReleaseProfileAPI.GetReleaseProfileById(r.auth, int32(profile.ID.ValueInt64())).Execute()
	if err != nil {
		if helpers.IsNotFound(err) {
			tflog.Warn(ctx, "removing "+releaseProfileResourceName+" from state, not found")
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Read, releaseProfileResourceName, err))

		return
	}

	tflog.Trace(ctx, "read "+releaseProfileResourceName+": "+strconv.Itoa(int(response.GetId())))
	// Map response body to resource schema attribute
	profile.write(ctx, response, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &profile)...)
}

func (r *ReleaseProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get plan values
	var profile *ReleaseProfile

	resp.Diagnostics.Append(req.Plan.Get(ctx, &profile)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Build Update resource
	request := profile.read(ctx, &resp.Diagnostics)

	if !r.keepUnset(request, &resp.Diagnostics) {
		return
	}

	// Update ReleaseProfile
	response, _, err := r.client.ReleaseProfileAPI.UpdateReleaseProfile(r.auth, strconv.Itoa(int(request.GetId()))).ReleaseProfileResource(*request).Execute()
	if err != nil {
		resp.Diagnostics.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Update, releaseProfileResourceName, err))

		return
	}

	tflog.Trace(ctx, "updated "+releaseProfileResourceName+": "+strconv.Itoa(int(response.GetId())))
	// Generate resource state struct
	profile.write(ctx, response, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &profile)...)
}

func (r *ReleaseProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var ID int64

	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &ID)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete releaseprofile current value
	_, err := r.client.ReleaseProfileAPI.DeleteReleaseProfile(r.auth, int32(ID)).Execute()
	if err != nil && !helpers.IsNotFound(err) {
		resp.Diagnostics.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Delete, releaseProfileResourceName, err))

		return
	}

	tflog.Trace(ctx, "deleted "+releaseProfileResourceName+": "+strconv.Itoa(int(ID)))
	resp.State.RemoveResource(ctx)
}

func (r *ReleaseProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportStatePassthroughIntID(ctx, path.Root("id"), req, resp)
	tflog.Trace(ctx, "imported "+releaseProfileResourceName+": "+req.ID)
}

func (p *ReleaseProfile) write(ctx context.Context, profile *sonarr.ReleaseProfileResource, diags *diag.Diagnostics) {
	var tempDiag diag.Diagnostics

	p.ID = types.Int64Value(int64(profile.GetId()))
	p.Name = types.StringValue(profile.GetName())
	p.Enabled = types.BoolValue(profile.GetEnabled())
	p.IndexerID = types.Int64Value(int64(profile.GetIndexerId()))
	p.Required, tempDiag = types.SetValueFrom(ctx, types.StringType, profile.GetRequired())
	diags.Append(tempDiag...)
	p.Ignored, tempDiag = types.SetValueFrom(ctx, types.StringType, profile.GetIgnored())
	diags.Append(tempDiag...)
	p.Tags, tempDiag = types.SetValueFrom(ctx, types.Int64Type, profile.GetTags())
	diags.Append(tempDiag...)

	// Sonarr v5+ fields: from Sonarr when it has them, else kept (v4 doesn't know them).
	p.AirDateRestriction = keepOrReadBool(p.AirDateRestriction, profile.GetAirDateRestrictionOk)
	p.AirDateGracePeriod = keepOrReadInt32(p.AirDateGracePeriod, profile.GetAirDateGracePeriodOk)
	p.AllowSeasonPackNotAired = keepOrReadBool(p.AllowSeasonPackNotAired, profile.GetAllowSeasonPackWithoutAllEpisodesAiredOk)

	if profile.HasExcludedTags() {
		p.ExcludedTags, tempDiag = types.SetValueFrom(ctx, types.Int64Type, profile.GetExcludedTags())
		diags.Append(tempDiag...)
	} else {
		// Sonarr v4 has none: keep the plan's; a data source (no plan) or unknown gets a typed null.
		if p.ExcludedTags.IsNull() || p.ExcludedTags.IsUnknown() {
			p.ExcludedTags = types.SetNull(types.Int64Type)
		}
	}
}

func (p *ReleaseProfile) read(ctx context.Context, diags *diag.Diagnostics) *sonarr.ReleaseProfileResource {
	profile := sonarr.NewReleaseProfileResource()
	profile.SetEnabled(p.Enabled.ValueBool())
	profile.SetId(int32(p.ID.ValueInt64()))
	profile.SetIndexerId(int32(p.IndexerID.ValueInt64()))
	profile.SetName(p.Name.ValueString())
	diags.Append(p.Tags.ElementsAs(ctx, &profile.Tags, true)...)
	diags.Append(p.Required.ElementsAs(ctx, &profile.Required, true)...)
	diags.Append(p.Ignored.ElementsAs(ctx, &profile.Ignored, true)...)

	// Sonarr v5+ fields: only what's configured; the rest comes from Sonarr (keepUnset).
	setBoolIfKnown(p.AirDateRestriction, profile.SetAirDateRestriction)
	setInt32IfKnown(p.AirDateGracePeriod, profile.SetAirDateGracePeriod)
	setBoolIfKnown(p.AllowSeasonPackNotAired, profile.SetAllowSeasonPackWithoutAllEpisodesAired)

	if isKnown(p.ExcludedTags) {
		excluded := make([]int32, 0, len(p.ExcludedTags.Elements()))
		diags.Append(p.ExcludedTags.ElementsAs(ctx, &excluded, true)...)
		profile.SetExcludedTags(excluded)
	}

	return profile
}

// keepUnset fills the Sonarr v5+ fields the plan leaves unset with the profile's current values:
// v5 resets a field a PUT leaves out. v4 has none of them.
func (r *ReleaseProfileResource) keepUnset(request *sonarr.ReleaseProfileResource, diags *diag.Diagnostics) bool {
	current, _, err := r.client.ReleaseProfileAPI.GetReleaseProfileById(r.auth, request.GetId()).Execute()
	if err != nil {
		diags.AddError(helpers.ClientError, helpers.ParseClientError(helpers.Read, releaseProfileResourceName, err))

		return false
	}

	fillReleaseProfileFromCurrent(request, current)

	return true
}

func fillReleaseProfileFromCurrent(request, current *sonarr.ReleaseProfileResource) {
	if v, ok := current.GetAirDateRestrictionOk(); ok && !request.HasAirDateRestriction() {
		request.SetAirDateRestriction(*v)
	}

	if v, ok := current.GetAirDateGracePeriodOk(); ok && !request.HasAirDateGracePeriod() {
		request.SetAirDateGracePeriod(*v)
	}

	if v, ok := current.GetAllowSeasonPackWithoutAllEpisodesAiredOk(); ok && !request.HasAllowSeasonPackWithoutAllEpisodesAired() {
		request.SetAllowSeasonPackWithoutAllEpisodesAired(*v)
	}

	if v, ok := current.GetExcludedTagsOk(); ok && !request.HasExcludedTags() {
		request.SetExcludedTags(v)
	}
}
