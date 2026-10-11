package main

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type backupResource struct{ client *Client }

func NewBackupResource() resource.Resource { return &backupResource{} }

type backupModel struct {
	ID              types.String `tfsdk:"id"`
	VMID            types.String `tfsdk:"vm_id"`
	Datastore       types.String `tfsdk:"datastore"`
	TargetHostID    types.String `tfsdk:"target_host_id"`
	TargetDatastore types.String `tfsdk:"target_datastore"`
	Triggers        types.Map    `tfsdk:"triggers"`
	HostID          types.String `tfsdk:"host_id"`
	BackupDatastore types.String `tfsdk:"backup_datastore"`
	VMName          types.String `tfsdk:"vm_name"`
	Stamp           types.String `tfsdk:"stamp"`
	Created         types.String `tfsdk:"created"`
	SizeBytes       types.Int64  `tfsdk:"size_bytes"`
}

func (r *backupResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_backup"
}

func (r *backupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A backup of a virtual machine, taken when the resource is created: a consolidated copy of its disks " +
			"and configuration in the hostwarden-backups folder of a datastore. A running VM is backed up through a " +
			"temporary snapshot. Destroying the resource deletes the backup. For recurring backups use hostwarden_schedule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "<host_id>/<datastore>/<vm_name>/<stamp>: usable as restore_from of hostwarden_vm.",
				Computed:      true,
				PlanModifiers: keep,
			},
			"vm_id": schema.StringAttribute{
				Description:   "The virtual machine to back up (hostwarden_vm id).",
				Required:      true,
				PlanModifiers: replace,
			},
			"datastore": schema.StringAttribute{
				Description:   "Datastore of the VM's host the backup is written to.",
				Required:      true,
				PlanModifiers: replace,
			},
			"target_host_id": schema.StringAttribute{
				Description:   "Move the backup to this other host (optional).",
				Optional:      true,
				PlanModifiers: replace,
			},
			"target_datastore": schema.StringAttribute{
				Description:   "Datastore on target_host_id.",
				Optional:      true,
				PlanModifiers: replace,
			},
			"triggers": schema.MapAttribute{
				Description:   "Arbitrary values: changing any of them takes a new backup (and deletes this one).",
				ElementType:   types.StringType,
				Optional:      true,
				PlanModifiers: []planmodifier.Map{mapRequiresReplace{}},
			},
			"host_id":          schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Host that holds the backup."},
			"backup_datastore": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Datastore that holds the backup."},
			"vm_name":          schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Name of the VM when it was backed up."},
			"stamp":            schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Time stamp of the backup (YYYYMMDD-HHMMSS, UTC)."},
			"created":          schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Creation time (RFC 3339)."},
			"size_bytes":       schema.Int64Attribute{Computed: true, Description: "Size of the backup."},
		},
	}
}

// mapRequiresReplace replaces the resource when the map changes.
type mapRequiresReplace struct{}

func (mapRequiresReplace) Description(context.Context) string {
	return "forces replacement when changed"
}
func (m mapRequiresReplace) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (mapRequiresReplace) PlanModifyMap(_ context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	resp.RequiresReplace = !req.PlanValue.Equal(req.StateValue)
}

func (r *backupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *backupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg backupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.TargetHostID.IsNull() != cfg.TargetDatastore.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("target_datastore"), "incomplete backup target",
			"Set target_host_id and target_datastore together.")
	}
}

func setBackupState(m *backupModel, b *apiBackup) {
	id := backupID{HostID: b.HostID, Datastore: b.Datastore, VMName: b.VMName, Stamp: b.Stamp}
	m.ID = types.StringValue(id.String())
	m.HostID = types.StringValue(b.HostID)
	m.BackupDatastore = types.StringValue(b.Datastore)
	m.VMName = types.StringValue(b.VMName)
	m.Stamp = types.StringValue(b.Stamp)
	m.Created = types.StringValue(b.Created.UTC().Format(time.RFC3339))
	m.SizeBytes = types.Int64Value(b.SizeBytes)
}

func (r *backupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(plan.VMID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid vm_id", err.Error())
		return
	}
	vm, err := r.client.GetVM(ctx, hostID, vmID)
	if err != nil {
		resp.Diagnostics.AddError("reading VM failed", err.Error())
		return
	}
	// Where the backup ends up: on the VM's host, or on the target host.
	dest := backupID{HostID: hostID, Datastore: plan.Datastore.ValueString(), VMName: vm.Name}
	if target := plan.TargetHostID.ValueString(); target != "" && target != hostID {
		dest.HostID, dest.Datastore = target, plan.TargetDatastore.ValueString()
	}
	before, err := r.client.ListBackups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing backups failed", err.Error())
		return
	}
	existing := map[string]bool{}
	for _, b := range before {
		existing[backupID{HostID: b.HostID, Datastore: b.Datastore, VMName: b.VMName, Stamp: b.Stamp}.String()] = true
	}

	job, err := r.client.BackupVM(withTaskTarget(ctx, vm.Name), hostID, vmID, apiBackupRequest{
		Datastore:       plan.Datastore.ValueString(),
		TargetHostID:    plan.TargetHostID.ValueString(),
		TargetDatastore: plan.TargetDatastore.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("backup failed", err.Error())
		return
	}
	jobWarnings(job, &resp.Diagnostics)

	// The job does not name the backup it made: it is the newest one at the
	// destination that was not there before.
	after, err := r.client.ListBackups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing backups failed", "The backup was taken, but the backups could not be listed: "+err.Error())
		return
	}
	var made *apiBackup
	for i := range after {
		b := &after[i]
		id := backupID{HostID: b.HostID, Datastore: b.Datastore, VMName: b.VMName, Stamp: b.Stamp}
		if b.HostID != dest.HostID || b.Datastore != dest.Datastore || b.VMName != dest.VMName || existing[id.String()] {
			continue
		}
		if made == nil || b.Stamp > made.Stamp {
			made = b
		}
	}
	if made == nil {
		resp.Diagnostics.AddError("backup not found", "The backup job completed, but no new backup of "+vm.Name+" is listed on the destination.")
		return
	}
	setBackupState(&plan, made)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.client.ListBackups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing backups failed", err.Error())
		return
	}
	for i := range list {
		b := &list[i]
		id := backupID{HostID: b.HostID, Datastore: b.Datastore, VMName: b.VMName, Stamp: b.Stamp}
		if id.String() == state.ID.ValueString() {
			setBackupState(&state, b)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// The listing skips hosts that cannot be reached: a backup on one of
	// those is not gone, so only forget it when its host answered.
	id, err := parseBackupID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	host, err := r.client.Host(ctx, id.HostID)
	if err == nil && (host.Status == nil || !host.Status.Connected) {
		resp.Diagnostics.AddWarning("backup host not reachable", "Host "+host.Name+" is not connected: the backup is kept in the state as it was.")
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update is never called: every configurable attribute requires replacement.
func (r *backupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan backupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseBackupID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	if err := r.client.DeleteBackup(ctx, id); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting backup failed", err.Error())
	}
}

var (
	_ resource.Resource                   = (*backupResource)(nil)
	_ resource.ResourceWithValidateConfig = (*backupResource)(nil)
)
