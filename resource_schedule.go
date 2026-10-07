package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type scheduleResource struct{ client *Client }

func NewScheduleResource() resource.Resource { return &scheduleResource{} }

type scheduleModel struct {
	ID                    types.String `tfsdk:"id"`
	VMID                  types.String `tfsdk:"vm_id"`
	Kind                  types.String `tfsdk:"kind"`
	Frequency             types.String `tfsdk:"frequency"`
	Hour                  types.Int64  `tfsdk:"hour"`
	Minute                types.Int64  `tfsdk:"minute"`
	Weekday               types.Int64  `tfsdk:"weekday"`
	Timezone              types.String `tfsdk:"timezone"`
	Keep                  types.Int64  `tfsdk:"keep"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	BackupDatastore       types.String `tfsdk:"backup_datastore"`
	BackupTargetHostID    types.String `tfsdk:"backup_target_host_id"`
	BackupTargetDatastore types.String `tfsdk:"backup_target_datastore"`
}

func (r *scheduleResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "freecenter_schedule"
}

func (r *scheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Scheduled snapshots or backups of a virtual machine, run by the FreeCenter server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vm_id": schema.StringAttribute{
				Description: "The virtual machine (freecenter_vm id).",
				Required:    true,
			},
			"kind": schema.StringAttribute{
				Description: "snapshot or backup.",
				Required:    true,
				Validators:  []validator.String{oneOf("snapshot", "backup")},
			},
			"frequency": schema.StringAttribute{
				Description: "hourly (at minute), daily (at hour:minute) or weekly (on weekday at hour:minute).",
				Required:    true,
				Validators:  []validator.String{oneOf("hourly", "daily", "weekly")},
			},
			"hour": schema.Int64Attribute{
				Description: "Hour of the day, 0-23 (default 0). Used by daily and weekly.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators:  []validator.Int64{int64Between(0, 23)},
			},
			"minute": schema.Int64Attribute{
				Description: "Minute, 0-59 (default 0).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators:  []validator.Int64{int64Between(0, 59)},
			},
			"weekday": schema.Int64Attribute{
				Description: "Day of the week, 0 = Sunday ... 6 = Saturday (default 0). Used by weekly.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators:  []validator.Int64{int64Between(0, 6)},
			},
			"timezone": schema.StringAttribute{
				Description: "IANA time zone the times are meant in, e.g. Europe/Rome (default UTC).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("UTC"),
			},
			"keep": schema.Int64Attribute{
				Description: "Scheduled snapshots or backups to keep; older ones are removed (default 0 = all).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators:  []validator.Int64{int64Between(0, 365)},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the schedule runs (default true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"backup_datastore": schema.StringAttribute{
				Description: "Kind backup: datastore of the VM's host the backup is written to.",
				Optional:    true,
			},
			"backup_target_host_id": schema.StringAttribute{
				Description: "Kind backup: move each backup to this other host (optional).",
				Optional:    true,
			},
			"backup_target_datastore": schema.StringAttribute{
				Description: "Kind backup: datastore on backup_target_host_id.",
				Optional:    true,
			},
		},
	}
}

func (r *scheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *scheduleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg scheduleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !known(cfg.Kind) {
		return
	}
	if cfg.Kind.ValueString() == "backup" {
		if cfg.BackupDatastore.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("backup_datastore"), "missing backup destination", "A backup schedule needs backup_datastore.")
		}
		if cfg.BackupTargetHostID.IsNull() != cfg.BackupTargetDatastore.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("backup_target_datastore"), "incomplete backup target",
				"Set backup_target_host_id and backup_target_datastore together.")
		}
		return
	}
	for name, v := range map[string]types.String{
		"backup_datastore":        cfg.BackupDatastore,
		"backup_target_host_id":   cfg.BackupTargetHostID,
		"backup_target_datastore": cfg.BackupTargetDatastore,
	} {
		if !v.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "conflicting attributes", name+" only applies to a backup schedule.")
		}
	}
}

// save creates or replaces the schedule on the server and stores its id.
func (r *scheduleResource) save(ctx context.Context, m *scheduleModel, id string) error {
	hostID, vmID, err := parseVMID(m.VMID.ValueString())
	if err != nil {
		return err
	}
	sc := apiSchedule{
		ID:        id,
		HostID:    hostID,
		VMID:      vmID,
		Kind:      m.Kind.ValueString(),
		Frequency: m.Frequency.ValueString(),
		Hour:      int(m.Hour.ValueInt64()),
		Minute:    int(m.Minute.ValueInt64()),
		Weekday:   int(m.Weekday.ValueInt64()),
		Timezone:  m.Timezone.ValueString(),
		Keep:      int(m.Keep.ValueInt64()),
		Enabled:   m.Enabled.ValueBool(),
	}
	if sc.Kind == "backup" {
		sc.Backup = &apiBackupRequest{
			Datastore:       m.BackupDatastore.ValueString(),
			TargetHostID:    m.BackupTargetHostID.ValueString(),
			TargetDatastore: m.BackupTargetDatastore.ValueString(),
		}
	}
	saved, err := r.client.PutSchedule(ctx, sc)
	if err != nil {
		return err
	}
	m.ID = types.StringValue(saved.ID)
	return nil
}

func (r *scheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.save(ctx, &plan, ""); err != nil {
		resp.Diagnostics.AddError("creating schedule failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.client.ListSchedules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing schedules failed", err.Error())
		return
	}
	for _, sc := range list {
		if sc.ID != state.ID.ValueString() {
			continue
		}
		state.VMID = types.StringValue(sc.HostID + "/" + sc.VMID)
		state.Kind = types.StringValue(sc.Kind)
		state.Frequency = types.StringValue(sc.Frequency)
		state.Hour = types.Int64Value(int64(sc.Hour))
		state.Minute = types.Int64Value(int64(sc.Minute))
		state.Weekday = types.Int64Value(int64(sc.Weekday))
		state.Timezone = types.StringValue(sc.Timezone)
		state.Keep = types.Int64Value(int64(sc.Keep))
		state.Enabled = types.BoolValue(sc.Enabled)
		backup := apiBackupRequest{}
		if sc.Backup != nil {
			backup = *sc.Backup
		}
		state.BackupDatastore = stringOrNull(backup.Datastore, state.BackupDatastore)
		state.BackupTargetHostID = stringOrNull(backup.TargetHostID, state.BackupTargetHostID)
		state.BackupTargetDatastore = stringOrNull(backup.TargetDatastore, state.BackupTargetDatastore)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *scheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.save(ctx, &plan, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("updating schedule failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSchedule(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting schedule failed", err.Error())
	}
}

func (r *scheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

var (
	_ resource.ResourceWithImportState    = (*scheduleResource)(nil)
	_ resource.ResourceWithValidateConfig = (*scheduleResource)(nil)
)
