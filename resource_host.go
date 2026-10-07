package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type hostResource struct {
	client *Client
}

func NewHostResource() resource.Resource { return &hostResource{} }

type hostResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Address         types.String `tfsdk:"address"`
	Username        types.String `tfsdk:"username"`
	Password        types.String `tfsdk:"password"`
	SSHEnabled      types.Bool   `tfsdk:"ssh_enabled"`
	SSHPort         types.Int64  `tfsdk:"ssh_port"`
	Insecure        types.Bool   `tfsdk:"insecure"`
	MaintenanceMode types.Bool   `tfsdk:"maintenance_mode"`
}

func (r *hostResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "freecenter_host"
}

func (r *hostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Register an ESXi host in FreeCenter.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Friendly host name.",
				Required:    true,
			},
			"address": schema.StringAttribute{
				Description: "ESXi address (host or host:port).",
				Required:    true,
			},
			"username": schema.StringAttribute{
				Description: "ESXi username (usually root).",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "ESXi password. FreeCenter stores it encrypted and never returns it.",
				Required:    true,
				Sensitive:   true,
			},
			"ssh_enabled": schema.BoolAttribute{
				Description: "Enable the SSH transport, required for every change on ESXi free (default true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"ssh_port": schema.Int64Attribute{
				Description: "SSH port (default 22).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(22),
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS verification of the ESXi API, for self-signed certificates (default true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"maintenance_mode": schema.BoolAttribute{
				Description: "Put the host in maintenance mode. Entering it fails while virtual machines are running: " +
					"power them off or migrate them first. When omitted the mode is left as it is.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *hostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func hostRequest(m *hostResourceModel) createHostRequest {
	return createHostRequest{
		Name:       m.Name.ValueString(),
		Address:    m.Address.ValueString(),
		Username:   m.Username.ValueString(),
		Password:   m.Password.ValueString(),
		SSHEnabled: m.SSHEnabled.ValueBool(),
		SSHPort:    int(m.SSHPort.ValueInt64()),
		Insecure:   m.Insecure.ValueBool(),
	}
}

// setHostState fills m with the host as FreeCenter reports it. The password
// is never returned by the API and keeps its value.
func setHostState(m *hostResourceModel, h *apiHost) {
	m.ID = types.StringValue(h.ID)
	m.Name = types.StringValue(h.Name)
	m.Address = types.StringValue(h.Address)
	m.Username = types.StringValue(h.Username)
	m.SSHEnabled = types.BoolValue(h.SSHEnabled)
	port := int64(h.SSHPort)
	if port == 0 {
		port = 22
	}
	m.SSHPort = types.Int64Value(port)
	m.Insecure = types.BoolValue(h.Insecure == nil || *h.Insecure)
	switch {
	case h.Status != nil && h.Status.Connected:
		m.MaintenanceMode = types.BoolValue(h.Status.MaintenanceMode)
	case !known(m.MaintenanceMode):
		// An unreachable host does not tell: assume the normal mode.
		m.MaintenanceMode = types.BoolValue(false)
	}
}

// applyMaintenance switches the maintenance mode when the plan asks for one
// that differs from the current one, and returns the host as it is afterwards.
func (r *hostResource) applyMaintenance(ctx context.Context, id string, want types.Bool) (*apiHost, error) {
	host, err := r.client.Host(ctx, id)
	if err != nil {
		return nil, err
	}
	if !known(want) {
		return host, nil
	}
	if host.Status == nil || !host.Status.Connected {
		return nil, fmt.Errorf("host %s is not connected: its maintenance mode cannot be read or changed", host.Name)
	}
	if host.Status.MaintenanceMode == want.ValueBool() {
		return host, nil
	}
	if err := r.client.SetMaintenance(withTaskTarget(ctx, host.Name), id, want.ValueBool()); err != nil {
		return nil, err
	}
	return r.client.Host(ctx, id)
}

func (r *hostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	host, err := r.client.CreateHost(ctx, hostRequest(&plan))
	if err != nil {
		resp.Diagnostics.AddError("creating host failed", err.Error())
		return
	}
	// The host is registered: record it even if the maintenance step fails.
	want := plan.MaintenanceMode
	if withStatus, err := r.applyMaintenance(ctx, host.ID, want); err != nil {
		resp.Diagnostics.AddError("setting the maintenance mode failed", err.Error())
		plan.MaintenanceMode = types.BoolValue(false)
	} else {
		host = withStatus
	}
	setHostState(&plan, host)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	host, err := r.client.Host(ctx, state.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("reading host failed", err.Error())
		return
	}
	setHostState(&state, host)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state hostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if _, err := r.client.UpdateHost(ctx, id, hostRequest(&plan)); err != nil {
		resp.Diagnostics.AddError("updating host failed", err.Error())
		return
	}
	host, err := r.applyMaintenance(ctx, id, plan.MaintenanceMode)
	if err != nil {
		resp.Diagnostics.AddError("setting the maintenance mode failed", err.Error())
		return
	}
	setHostState(&plan, host)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteHost(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting host failed", err.Error())
	}
}

var _ resource.Resource = (*hostResource)(nil)
