package main

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// importHostScoped imports a resource whose id is <host_id>/<name>.
func importHostScoped(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 2, "<host_id>/<name>")
	if err != nil {
		resp.Diagnostics.AddError("invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

// ---- hostwarden_port_group ----

type portGroupResource struct{ client *Client }

func NewPortGroupResource() resource.Resource { return &portGroupResource{} }

type portGroupModel struct {
	ID      types.String `tfsdk:"id"`
	HostID  types.String `tfsdk:"host_id"`
	Name    types.String `tfsdk:"name"`
	VSwitch types.String `tfsdk:"vswitch"`
	VLAN    types.Int64  `tfsdk:"vlan"`
}

func (r *portGroupResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_port_group"
}

func (r *portGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A virtual machine port group on a standard virtual switch of a host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<host_id>/<name>.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"name": schema.StringAttribute{
				Description:   "Port group name: the network name virtual machines connect to.",
				Required:      true,
				PlanModifiers: replace,
			},
			"vswitch": schema.StringAttribute{
				Description:   "Standard virtual switch, e.g. vSwitch0.",
				Required:      true,
				PlanModifiers: replace,
			},
			"vlan": schema.Int64Attribute{
				Description: "VLAN id: 0 none, 1-4094 tagged, 4095 trunk (default 0).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators:  []validator.Int64{int64Between(0, 4095)},
			},
		},
	}
}

func (r *portGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *portGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, name := plan.HostID.ValueString(), plan.Name.ValueString()
	err := r.client.AddPortGroup(ctx, hostID, portGroupRequest{Name: name, VSwitch: plan.VSwitch.ValueString(), VLAN: plan.VLAN.ValueInt64()})
	if err != nil {
		resp.Diagnostics.AddError("creating port group failed", err.Error())
		return
	}
	plan.ID = types.StringValue(hostID + "/" + name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	net, err := r.client.HostNetwork(ctx, state.HostID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("reading host network failed", err.Error())
		return
	}
	for _, pg := range net.PortGroups {
		if pg.Name == state.Name.ValueString() {
			state.VSwitch = types.StringValue(pg.VSwitch)
			state.VLAN = types.Int64Value(pg.VLAN)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *portGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only the VLAN changes in place.
	if err := r.client.SetPortGroupVLAN(ctx, plan.HostID.ValueString(), plan.Name.ValueString(), plan.VLAN.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("updating port group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.RemovePortGroup(ctx, state.HostID.ValueString(), state.Name.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting port group failed", err.Error())
	}
}

func (r *portGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHostScoped(ctx, req, resp)
}

// ---- hostwarden_vswitch ----

type vSwitchResource struct{ client *Client }

func NewVSwitchResource() resource.Resource { return &vSwitchResource{} }

type vSwitchModel struct {
	ID      types.String `tfsdk:"id"`
	HostID  types.String `tfsdk:"host_id"`
	Name    types.String `tfsdk:"name"`
	Uplinks types.Set    `tfsdk:"uplinks"`
	MTU     types.Int64  `tfsdk:"mtu"`
}

func (r *vSwitchResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_vswitch"
}

func (r *vSwitchResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A standard virtual switch of a host. It can only be destroyed once it has no port groups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<host_id>/<name>.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"name":    schema.StringAttribute{Required: true, PlanModifiers: replace},
			"uplinks": schema.SetAttribute{
				Description: "Physical adapters of the switch (e.g. vmnic1), not in use by another switch. " +
					"Without uplinks the switch is internal to the host.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown(), setplanmodifier.RequiresReplace()},
			},
			"mtu": schema.Int64Attribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *vSwitchResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// find returns the switch as the host reports it, or nil when it is not there.
func (r *vSwitchResource) find(ctx context.Context, hostID, name string) (*apiVSwitch, error) {
	net, err := r.client.HostNetwork(ctx, hostID)
	if err != nil {
		return nil, err
	}
	for i := range net.VSwitches {
		if net.VSwitches[i].Name == name {
			return &net.VSwitches[i], nil
		}
	}
	return nil, nil
}

func (r *vSwitchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vSwitchModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, name := plan.HostID.ValueString(), plan.Name.ValueString()
	uplinks := toStrings(ctx, plan.Uplinks, &resp.Diagnostics)
	sort.Strings(uplinks)
	if uplinks == nil {
		uplinks = []string{}
	}
	if err := r.client.AddVSwitch(ctx, hostID, vSwitchRequest{Name: name, Uplinks: uplinks}); err != nil {
		resp.Diagnostics.AddError("creating virtual switch failed", err.Error())
		return
	}
	plan.ID = types.StringValue(hostID + "/" + name)
	plan.Uplinks = stringSet(ctx, uplinks, &resp.Diagnostics)
	plan.MTU = types.Int64Value(1500)
	sw, err := r.find(ctx, hostID, name)
	if err != nil {
		resp.Diagnostics.AddWarning("reading the new virtual switch failed", err.Error())
	} else if sw != nil {
		plan.MTU = types.Int64Value(sw.MTU)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vSwitchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vSwitchModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sw, err := r.find(ctx, state.HostID.ValueString(), state.Name.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("reading host network failed", err.Error())
		return
	}
	if sw == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Uplinks = stringSet(ctx, sw.Uplinks, &resp.Diagnostics)
	state.MTU = types.Int64Value(sw.MTU)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: every configurable attribute requires replacement.
func (r *vSwitchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vSwitchModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vSwitchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vSwitchModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.RemoveVSwitch(ctx, state.HostID.ValueString(), state.Name.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting virtual switch failed", err.Error())
	}
}

func (r *vSwitchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHostScoped(ctx, req, resp)
}

var (
	_ resource.ResourceWithImportState = (*portGroupResource)(nil)
	_ resource.ResourceWithImportState = (*vSwitchResource)(nil)
)
