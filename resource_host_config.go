package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---- hostwarden_host_service ----

type hostServiceResource struct{ client *Client }

func NewHostServiceResource() resource.Resource { return &hostServiceResource{} }

type hostServiceModel struct {
	ID      types.String `tfsdk:"id"`
	HostID  types.String `tfsdk:"host_id"`
	Key     types.String `tfsdk:"key"`
	Running types.Bool   `tfsdk:"running"`
	Policy  types.String `tfsdk:"policy"`
	Label   types.String `tfsdk:"label"`
}

func (r *hostServiceResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_host_service"
}

func (r *hostServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "State and startup policy of a service of an ESXi host (e.g. ntpd, TSM for the ESXi Shell). " +
			"The service exists on the host regardless: destroying the resource leaves it as it is. " +
			"The SSH service cannot be stopped or disabled, as Hostwarden manages the host through it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<host_id>/<key>.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"key": schema.StringAttribute{
				Description:   "Service key as the host reports it, e.g. ntpd, TSM, TSM-SSH.",
				Required:      true,
				PlanModifiers: replace,
			},
			"running": schema.BoolAttribute{
				Description:   "Whether the service runs. When omitted the service is not started nor stopped.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"policy": schema.StringAttribute{
				Description:   "Startup policy: on (start with the host) or off. When omitted the policy is not changed.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:    []validator.String{oneOf("on", "off")},
			},
			"label": schema.StringAttribute{
				Description:   "Display name of the service.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *hostServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *hostServiceResource) find(ctx context.Context, hostID, key string) (*apiService, error) {
	services, err := r.client.ListServices(ctx, hostID)
	if err != nil {
		return nil, err
	}
	for i := range services {
		if services[i].Key == key {
			return &services[i], nil
		}
	}
	return nil, &apiError{Code: 404, Msg: "service " + key + " not found on the host"}
}

// apply brings the service to the planned state and stores what the host reports.
func (r *hostServiceResource) apply(ctx context.Context, plan *hostServiceModel) error {
	hostID, key := plan.HostID.ValueString(), plan.Key.ValueString()
	cur, err := r.find(ctx, hostID, key)
	if err != nil {
		return err
	}
	var req serviceRequest
	if known(plan.Running) && plan.Running.ValueBool() != cur.Running {
		req.Action = "stop"
		if plan.Running.ValueBool() {
			req.Action = "start"
		}
	}
	if known(plan.Policy) && plan.Policy.ValueString() != cur.Policy {
		req.Policy = plan.Policy.ValueString()
	}
	if req != (serviceRequest{}) {
		if err := r.client.SetService(ctx, hostID, key, req); err != nil {
			return err
		}
		if cur, err = r.find(ctx, hostID, key); err != nil {
			return err
		}
	}
	if known(plan.Running) && plan.Running.ValueBool() != cur.Running {
		return fmt.Errorf("service %s did not reach the requested state (running: %t)", key, cur.Running)
	}
	plan.ID = types.StringValue(hostID + "/" + key)
	plan.Running = types.BoolValue(cur.Running)
	plan.Policy = types.StringValue(cur.Policy)
	plan.Label = types.StringValue(cur.Label)
	return nil
}

func (r *hostServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("configuring service failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.find(ctx, state.HostID.ValueString(), state.Key.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("reading service failed", err.Error())
		return
	}
	state.Running = types.BoolValue(cur.Running)
	state.Policy = types.StringValue(cur.Policy)
	state.Label = types.StringValue(cur.Label)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("configuring service failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the service: it belongs to the host.
func (r *hostServiceResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

// ---- hostwarden_host_ntp ----

type hostNTPResource struct{ client *Client }

func NewHostNTPResource() resource.Resource { return &hostNTPResource{} }

type hostNTPModel struct {
	ID      types.String `tfsdk:"id"`
	HostID  types.String `tfsdk:"host_id"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Servers types.List   `tfsdk:"servers"`
}

func (r *hostNTPResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_host_ntp"
}

func (r *hostNTPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "NTP configuration of an ESXi host. Destroying the resource leaves the configuration on the host as it is.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "The host id.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Description: "Synchronise the host clock with the NTP servers (default true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"servers": schema.ListAttribute{
				Description: "NTP servers (names or addresses). At least one is required when enabled.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *hostNTPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *hostNTPResource) put(ctx context.Context, plan *hostNTPModel) error {
	var servers []string
	if diags := plan.Servers.ElementsAs(ctx, &servers, false); diags.HasError() {
		return fmt.Errorf("invalid servers: %s", diags.Errors()[0].Detail())
	}
	hostID := plan.HostID.ValueString()
	if err := r.client.SetHostTime(ctx, hostID, apiHostTime{NTPEnabled: plan.Enabled.ValueBool(), NTPServers: servers}); err != nil {
		return err
	}
	plan.ID = types.StringValue(hostID)
	return nil
}

func (r *hostNTPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostNTPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("configuring NTP failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostNTPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostNTPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.HostTime(ctx, state.HostID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("reading host time failed", err.Error())
		return
	}
	state.Enabled = types.BoolValue(t.NTPEnabled)
	state.Servers = stringList(ctx, t.NTPServers, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostNTPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostNTPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("configuring NTP failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the configuration: it belongs to the host.
func (r *hostNTPResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

var (
	_ resource.Resource = (*hostServiceResource)(nil)
	_ resource.Resource = (*hostNTPResource)(nil)
)
