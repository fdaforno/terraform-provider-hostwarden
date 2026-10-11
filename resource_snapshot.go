package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type snapshotResource struct{ client *Client }

func NewSnapshotResource() resource.Resource { return &snapshotResource{} }

type snapshotModel struct {
	ID          types.String `tfsdk:"id"`
	VMID        types.String `tfsdk:"vm_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Memory      types.Bool   `tfsdk:"memory"`
	Quiesce     types.Bool   `tfsdk:"quiesce"`
	SnapshotID  types.String `tfsdk:"snapshot_id"`
}

func (r *snapshotResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_vm_snapshot"
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	replaceBool := []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A snapshot of a virtual machine. Destroying the resource deletes the snapshot (its changes are merged into the disks).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<host_id>/<vm_id>/<snapshot_id>.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vm_id": schema.StringAttribute{
				Description:   "The virtual machine (hostwarden_vm id).",
				Required:      true,
				PlanModifiers: replaceString,
			},
			"name": schema.StringAttribute{
				Description:   "Snapshot name.",
				Required:      true,
				PlanModifiers: replaceString,
			},
			"description": schema.StringAttribute{
				Optional:      true,
				PlanModifiers: replaceString,
			},
			"memory": schema.BoolAttribute{
				Description:   "Include the memory of a running VM, so that reverting resumes it where it was.",
				Optional:      true,
				PlanModifiers: replaceBool,
			},
			"quiesce": schema.BoolAttribute{
				Description:   "Quiesce the guest file systems (requires the guest tools).",
				Optional:      true,
				PlanModifiers: replaceBool,
			},
			"snapshot_id": schema.StringAttribute{
				Description:   "Id of the snapshot on the host.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *snapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(plan.VMID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid vm_id", err.Error())
		return
	}
	before, err := r.client.ListSnapshots(ctx, hostID, vmID)
	if err != nil {
		resp.Diagnostics.AddError("listing snapshots failed", err.Error())
		return
	}
	existing := map[string]bool{}
	for _, s := range before {
		existing[s.ID] = true
	}
	name := plan.Name.ValueString()
	after, err := r.client.CreateSnapshot(withTaskTarget(ctx, name), hostID, vmID, createSnapshotRequest{
		Name:        name,
		Description: plan.Description.ValueString(),
		Memory:      plan.Memory.ValueBool(),
		Quiesce:     plan.Quiesce.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("creating snapshot failed", err.Error())
		return
	}
	// The API answers with every snapshot of the VM: the new one is the one
	// that was not there before.
	snapID := ""
	for _, s := range after {
		if !existing[s.ID] && s.Name == name {
			snapID = s.ID
		}
	}
	if snapID == "" {
		resp.Diagnostics.AddError("creating snapshot failed", "The snapshot was requested but is not among the snapshots of the VM.")
		return
	}
	plan.SnapshotID = types.StringValue(snapID)
	plan.ID = types.StringValue(hostID + "/" + vmID + "/" + snapID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(state.VMID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	snaps, err := r.client.ListSnapshots(ctx, hostID, vmID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("listing snapshots failed", err.Error())
		return
	}
	for _, s := range snaps {
		if s.ID == state.SnapshotID.ValueString() {
			state.Name = types.StringValue(s.Name)
			state.Description = stringOrNull(s.Description, state.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update is never called: every attribute requires replacement.
func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan snapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(state.VMID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	ctx = withTaskTarget(ctx, state.Name.ValueString())
	if err := r.client.DeleteSnapshot(ctx, hostID, vmID, state.SnapshotID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting snapshot failed", err.Error())
	}
}

var _ resource.Resource = (*snapshotResource)(nil)
