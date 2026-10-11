package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// powerWait bounds the wait for a VM to reach a power state.
const powerWait = 2 * time.Minute

type vmResource struct {
	client *Client
}

func NewVMResource() resource.Resource { return &vmResource{} }

type cloudInitUserModel struct {
	Name              types.String `tfsdk:"name"`
	Sudo              types.String `tfsdk:"sudo"`
	Shell             types.String `tfsdk:"shell"`
	Password          types.String `tfsdk:"password"`
	SSHAuthorizedKeys types.List   `tfsdk:"ssh_authorized_keys"`
}

type cloudInitFileModel struct {
	Path        types.String `tfsdk:"path"`
	Content     types.String `tfsdk:"content"`
	Permissions types.String `tfsdk:"permissions"`
	Owner       types.String `tfsdk:"owner"`
}

type cloudInitModel struct {
	Hostname          types.String `tfsdk:"hostname"`
	RootPassword      types.String `tfsdk:"root_password"`
	SSHAuthorizedKeys types.List   `tfsdk:"ssh_authorized_keys"`
	Users             types.List   `tfsdk:"users"`
	Packages          types.List   `tfsdk:"packages"`
	RunCmd            types.List   `tfsdk:"runcmd"`
	WriteFiles        types.List   `tfsdk:"write_files"`
	NetworkConfig     types.String `tfsdk:"network_config"`
	UserData          types.String `tfsdk:"user_data"`
}

type vmResourceModel struct {
	ID                 types.String    `tfsdk:"id"`
	HostID             types.String    `tfsdk:"host_id"`
	Name               types.String    `tfsdk:"name"`
	GuestID            types.String    `tfsdk:"guest_id"`
	Datastore          types.String    `tfsdk:"datastore"`
	NumCPUs            types.Int64     `tfsdk:"num_cpus"`
	CoresPerSocket     types.Int64     `tfsdk:"cores_per_socket"`
	MemoryMB           types.Int64     `tfsdk:"memory_mb"`
	DiskSizesGB        types.List      `tfsdk:"disk_sizes_gb"`
	DiskThin           types.Bool      `tfsdk:"disk_thin"`
	Network            types.String    `tfsdk:"network"`
	AdditionalNetworks types.List      `tfsdk:"additional_networks"`
	AdapterType        types.String    `tfsdk:"adapter_type"`
	SCSIController     types.String    `tfsdk:"scsi_controller"`
	Annotation         types.String    `tfsdk:"annotation"`
	ISOPath            types.String    `tfsdk:"iso_path"`
	BootFromCDROM      types.Bool      `tfsdk:"boot_from_cdrom"`
	VNCEnabled         types.Bool      `tfsdk:"vnc_enabled"`
	VNCPort            types.Int64     `tfsdk:"vnc_port"`
	VNCPassword        types.String    `tfsdk:"vnc_password"`
	CloudImage         types.String    `tfsdk:"cloud_image"`
	CloudInit          *cloudInitModel `tfsdk:"cloud_init"`
	CloneFrom          types.String    `tfsdk:"clone_from"`
	RestoreFrom        types.String    `tfsdk:"restore_from"`
	Firmware           types.String    `tfsdk:"firmware"`
	BootDelayMs        types.Int64     `tfsdk:"boot_delay_ms"`
	CPUHotAdd          types.Bool      `tfsdk:"cpu_hot_add"`
	MemoryHotAdd       types.Bool      `tfsdk:"memory_hot_add"`
	NestedHV           types.Bool      `tfsdk:"nested_hv"`
	USBController      types.String    `tfsdk:"usb_controller"`
	Template           types.Bool      `tfsdk:"template"`
	MigrateMode        types.String    `tfsdk:"migrate_mode"`
	MigratePrecopy     types.Bool      `tfsdk:"migrate_precopy"`
	MigrateReclaimThin types.Bool      `tfsdk:"migrate_reclaim_thin"`
	Power              types.String    `tfsdk:"power"`
	IPAddress          types.String    `tfsdk:"ip_address"`
	UUID               types.String    `tfsdk:"uuid"`
}

func (r *vmResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "freecenter_vm"
}

func (r *vmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	keepBool := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	keepList := []planmodifier.List{listplanmodifier.UseStateForUnknown()}
	keepOrReplaceString := []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}
	listOfStrings := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.StringType, Optional: true, Description: desc}
	}

	resp.Schema = schema.Schema{
		Description: "A virtual machine managed by FreeCenter on an ESXi host. Hardware and option changes are applied " +
			"with the VM powered off: the provider powers it off, applies them and restores the desired power state. " +
			"Changing host_id migrates the VM to the other host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "FreeCenter VM id (<host_id>/<vm_id>). It changes when the VM is migrated.",
				Computed:      true,
				PlanModifiers: keepString,
			},
			"host_id": schema.StringAttribute{
				Description: "FreeCenter host id (freecenter_host.id or data.freecenter_host.id). Changing it migrates the VM " +
					"to that host, on the datastore named by datastore: the files are copied directly between the two hosts.",
				Required: true,
			},
			"name": schema.StringAttribute{
				Description: "VM name. It names the folder on the datastore at creation; renaming changes the display name only.",
				Required:    true,
			},
			"guest_id": schema.StringAttribute{
				Description:   "Guest OS id, e.g. ubuntu64Guest. Required unless the VM is cloned or restored.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
			},
			"datastore": schema.StringAttribute{
				Description: "Datastore of the VM folder and disks. Changing it together with host_id chooses the datastore " +
					"on the target host; changing it alone replaces the VM.",
				Required: true,
			},
			"num_cpus": schema.Int64Attribute{
				Description:   "Number of virtual CPUs (default 2).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepInt,
				Validators:    []validator.Int64{int64Between(1, 128)},
			},
			"cores_per_socket": schema.Int64Attribute{
				Description:   "Cores per virtual socket.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepInt,
				Validators:    []validator.Int64{int64Between(1, 128)},
			},
			"memory_mb": schema.Int64Attribute{
				Description:   "Memory in MB (default 2048).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepInt,
			},
			"disk_sizes_gb": schema.ListAttribute{
				Description: "Disk sizes in GB. Disks can grow and be appended; shrinking and removing are not supported. " +
					"Optional with cloud_image, clone_from and restore_from.",
				ElementType:   types.Int64Type,
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepList,
			},
			"disk_thin": schema.BoolAttribute{
				Description: "Thin provisioning for new disks (default true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"network": schema.StringAttribute{
				Description:   "Port group of the first network adapter. It can be changed in place.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
			},
			"additional_networks": schema.ListAttribute{
				Description: "Port groups of the network adapters after the first, in order. Adapters are added, moved to " +
					"another port group and removed to match the list; when omitted they are left alone.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepList,
			},
			"adapter_type": schema.StringAttribute{
				Description:   "Type of the network adapters: e1000, e1000e or vmxnet3 (default e1000).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepOrReplaceString,
				Validators:    []validator.String{oneOf("e1000", "e1000e", "vmxnet3")},
			},
			"scsi_controller": schema.StringAttribute{
				Description:   "SCSI controller: lsilogic-sas, pvscsi, lsilogic or buslogic (default lsilogic-sas).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepOrReplaceString,
			},
			"annotation": schema.StringAttribute{
				Description: "VM notes.",
				Optional:    true,
			},
			"iso_path": schema.StringAttribute{
				Description: "ISO in the CD/DVD drive, e.g. [datastore] isos/ubuntu.iso; an empty string ejects it. " +
					"With cloud_init it is set automatically to the seed ISO.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
			},
			"boot_from_cdrom": schema.BoolAttribute{
				Description:   "Boot from the CD/DVD drive first, then from the hard disk. Requires iso_path.",
				Optional:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"vnc_enabled": schema.BoolAttribute{
				Description:   "Enable the remote VNC console of the VM.",
				Optional:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"vnc_port": schema.Int64Attribute{
				Description:   "VNC port (0 = assigned automatically). Requires vnc_enabled.",
				Optional:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"vnc_password": schema.StringAttribute{
				Description:   "VNC password. Requires vnc_enabled.",
				Optional:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cloud_image": schema.StringAttribute{
				Description: "Cloud image on the datastore (e.g. cloud-images/rocky-9.vmdk): the VM boots from a copy of " +
					"this installed image instead of an empty disk.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"clone_from": schema.StringAttribute{
				Description: "Create the VM as a clone of this VM or template (its freecenter_vm id). The source must be on " +
					"host_id and powered off. The other attributes are then applied to the clone.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"restore_from": schema.StringAttribute{
				Description: "Create the VM from this backup (a freecenter_backup id). The backup must be on host_id. " +
					"The other attributes are then applied to the restored VM.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"firmware": schema.StringAttribute{
				Description:   "Firmware: bios or efi.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
				Validators:    []validator.String{oneOf("bios", "efi")},
			},
			"boot_delay_ms": schema.Int64Attribute{
				Description:   "Delay before the firmware starts booting, in milliseconds (0-10000).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepInt,
				Validators:    []validator.Int64{int64Between(0, 10000)},
			},
			"cpu_hot_add": schema.BoolAttribute{
				Description:   "Allow adding virtual CPUs while the VM runs.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepBool,
			},
			"memory_hot_add": schema.BoolAttribute{
				Description:   "Allow adding memory while the VM runs.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepBool,
			},
			"nested_hv": schema.BoolAttribute{
				Description:   "Expose hardware-assisted virtualization to the guest (nested hypervisors).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepBool,
			},
			"usb_controller": schema.StringAttribute{
				Description:   "USB controller: usb2, usb3 or an empty string for none.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
				Validators:    []validator.String{oneOf("", "usb2", "usb3")},
			},
			"template": schema.BoolAttribute{
				Description:   "Mark the VM as a template: it cannot be powered on and is meant to be cloned (clone_from).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepBool,
			},
			"migrate_mode": schema.StringAttribute{
				Description: "How a running VM is migrated when host_id changes: stop_start (shut down, copy, power on) or " +
					"suspend (suspend, copy, resume: hosts with the same CPU only); cold for a powered-off VM. " +
					"By default FreeCenter picks the mode from the power state and the CPUs of the two hosts.",
				Optional:   true,
				Validators: []validator.String{oneOf("cold", "stop_start", "suspend")},
			},
			"migrate_precopy": schema.BoolAttribute{
				Description: "Migration of a running VM: copy the disks from a temporary snapshot before stopping the VM, " +
					"to shorten the downtime (default false).",
				Optional: true,
			},
			"migrate_reclaim_thin": schema.BoolAttribute{
				Description: "Migration: make the copied disks thin again on the target host (default true).",
				Optional:    true,
			},
			"power": schema.StringAttribute{
				Description:   "Desired power state: on or off (default on; off for a template).",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keepString,
				Validators:    []validator.String{oneOf("on", "off")},
			},
			"ip_address": schema.StringAttribute{
				Description: "Primary IP address reported by the guest tools, when known.",
				Computed:    true,
			},
			"uuid": schema.StringAttribute{
				Description: "BIOS UUID of the VM. It can be empty until the VM is powered on for the first time.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"cloud_init": schema.SingleNestedBlock{
				Description: "Cloud-init configuration, applied on the first boot of a VM created from cloud_image. " +
					"Later changes do not reach an existing VM.",
				Attributes: map[string]schema.Attribute{
					"hostname":            schema.StringAttribute{Optional: true, Description: "Hostname of the VM."},
					"root_password":       schema.StringAttribute{Optional: true, Sensitive: true, Description: "Root password (plain text, set on first boot)."},
					"ssh_authorized_keys": listOfStrings("SSH public keys of the default user."),
					"packages":            listOfStrings("Packages to install on first boot."),
					"runcmd":              listOfStrings("Commands to run on first boot."),
					"network_config":      schema.StringAttribute{Optional: true, Description: "Network configuration (cloud-init network-config YAML, version 2), e.g. for a static address."},
					"user_data":           schema.StringAttribute{Optional: true, Description: "Raw cloud-init user-data YAML. Overrides every other field except network_config."},
					"users": schema.ListNestedAttribute{
						Optional:    true,
						Description: "Users to create.",
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"name":                schema.StringAttribute{Required: true, Description: "User name."},
							"sudo":                schema.StringAttribute{Optional: true, Description: "Sudo rule, e.g. ALL=(ALL) NOPASSWD:ALL."},
							"shell":               schema.StringAttribute{Optional: true, Description: "Login shell."},
							"password":            schema.StringAttribute{Optional: true, Sensitive: true, Description: "Password (plain text)."},
							"ssh_authorized_keys": listOfStrings("SSH public keys of the user."),
						}},
					},
					"write_files": schema.ListNestedAttribute{
						Optional:    true,
						Description: "Files to write on first boot.",
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"path":        schema.StringAttribute{Required: true, Description: "Absolute path of the file."},
							"content":     schema.StringAttribute{Required: true, Description: "Content of the file."},
							"permissions": schema.StringAttribute{Optional: true, Description: "Mode, e.g. 0644."},
							"owner":       schema.StringAttribute{Optional: true, Description: "Owner, e.g. root:root."},
						}},
					},
				},
			},
		},
	}
}

func (r *vmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *vmResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg vmResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	source := ""
	switch {
	case !cfg.CloneFrom.IsNull() && !cfg.RestoreFrom.IsNull():
		resp.Diagnostics.AddAttributeError(path.Root("restore_from"), "conflicting attributes", "Set clone_from or restore_from, not both.")
		return
	case !cfg.CloneFrom.IsNull():
		source = "clone_from"
	case !cfg.RestoreFrom.IsNull():
		source = "restore_from"
	}
	if source == "" {
		if cfg.GuestID.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("guest_id"), "missing guest_id", "guest_id is required unless clone_from or restore_from is set.")
		}
	} else {
		// These only make sense for a VM built from scratch.
		creationOnly := map[string]bool{
			"cloud_image":     !cfg.CloudImage.IsNull(),
			"cloud_init":      cfg.CloudInit != nil,
			"boot_from_cdrom": !cfg.BootFromCDROM.IsNull(),
			"scsi_controller": !cfg.SCSIController.IsNull(),
			"vnc_enabled":     !cfg.VNCEnabled.IsNull(),
		}
		for name, set := range creationOnly {
			if set {
				resp.Diagnostics.AddAttributeError(path.Root(name), "conflicting attributes", name+" cannot be used with "+source+".")
			}
		}
	}
	if known(cfg.Template) && cfg.Template.ValueBool() && known(cfg.Power) && cfg.Power.ValueString() == "on" {
		resp.Diagnostics.AddAttributeError(path.Root("power"), "a template cannot be powered on", `Remove power or set it to "off" when template is true.`)
	}
}

// ModifyPlan handles what depends on more than one attribute: the datastore
// may only change with the host (migration), and a template is powered off.
func (r *vmResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy
	}
	var plan, cfg vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Power.IsNull() && known(plan.Template) && plan.Template.ValueBool() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("power"), types.StringValue("off"))...)
	}
	if req.State.Raw.IsNull() {
		return // create
	}
	var state vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	switch {
	case !plan.HostID.Equal(state.HostID):
		// Migration: the VM gets a new id on the target host.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
	case known(plan.Datastore) && !plan.Datastore.Equal(state.Datastore):
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("datastore"))
	}
}

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, name := plan.HostID.ValueString(), plan.Name.ValueString()
	ctx = withTaskTarget(ctx, name)

	// The VM is always created powered off: the options the creation request
	// does not carry are applied before its first boot.
	var vmID string
	switch {
	case known(plan.CloneFrom):
		srcHost, srcVM, err := parseVMID(plan.CloneFrom.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("clone_from"), "invalid clone_from", err.Error())
			return
		}
		if srcHost != hostID {
			resp.Diagnostics.AddAttributeError(path.Root("clone_from"), "clone source on another host",
				"A VM is cloned on the host of its source. Clone it there, then change host_id to migrate the clone.")
			return
		}
		job, err := r.client.CloneVM(ctx, srcHost, srcVM, cloneRequest{Name: name, Datastore: plan.Datastore.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("cloning VM failed", err.Error())
			return
		}
		vmID = job.NewVMID
		jobWarnings(job, &resp.Diagnostics)
	case known(plan.RestoreFrom):
		backup, err := parseBackupID(plan.RestoreFrom.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("restore_from"), "invalid restore_from", err.Error())
			return
		}
		if backup.HostID != hostID {
			resp.Diagnostics.AddAttributeError(path.Root("restore_from"), "backup on another host",
				"A backup is restored on the host that holds it. Restore it there, then change host_id to migrate the VM.")
			return
		}
		job, err := r.client.RestoreBackup(ctx, backup, restoreRequest{Name: name, Datastore: plan.Datastore.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("restoring VM failed", err.Error())
			return
		}
		vmID = job.NewVMID
		jobWarnings(job, &resp.Diagnostics)
	default:
		vm, err := r.client.CreateVM(ctx, hostID, createVMRequest{
			Name:           name,
			GuestID:        plan.GuestID.ValueString(),
			NumCPUs:        int(attrInt64(plan.NumCPUs, 2)),
			CoresPerSocket: int(attrInt64(plan.CoresPerSocket, 0)),
			MemoryMB:       attrInt64(plan.MemoryMB, 2048),
			Datastore:      plan.Datastore.ValueString(),
			Disks:          diskSpecs(toInt64s(ctx, plan.DiskSizesGB, &resp.Diagnostics), plan.DiskThin.ValueBool()),
			Network:        plan.Network.ValueString(),
			AdapterType:    attrString(plan.AdapterType, "e1000"),
			SCSIController: attrString(plan.SCSIController, "lsilogic-sas"),
			Annotation:     plan.Annotation.ValueString(),
			ISOPath:        plan.ISOPath.ValueString(),
			BootFromCDROM:  plan.BootFromCDROM.ValueBool(),
			VNCEnabled:     plan.VNCEnabled.ValueBool(),
			VNCPort:        int(attrInt64(plan.VNCPort, 0)),
			VNCPassword:    plan.VNCPassword.ValueString(),
			CloudImage:     plan.CloudImage.ValueString(),
			CloudInit:      toCloudInitAPI(ctx, plan.CloudInit, &resp.Diagnostics),
		})
		if err != nil {
			resp.Diagnostics.AddError("creating VM failed", err.Error())
			return
		}
		vmID = vm.ID
	}
	if vmID == "" {
		resp.Diagnostics.AddError("creating VM failed", "FreeCenter did not return the id of the new VM.")
		return
	}

	// From here on the VM exists: it is recorded even if a later step fails.
	vm, err := r.reconcile(ctx, hostID, vmID, &plan, &resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("configuring the new VM failed", err.Error())
		if vm, err = r.client.GetVM(ctx, hostID, vmID); err != nil {
			resp.Diagnostics.AddError("reading the new VM failed", fmt.Sprintf("VM %s/%s was created but could not be read: %s", hostID, vmID, err))
			return
		}
	}
	setVMState(ctx, &plan, hostID, vm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func jobWarnings(job *apiJob, diags *diag.Diagnostics) {
	for _, w := range job.Warnings {
		diags.AddWarning(job.Kind+" of "+job.VMName, w)
	}
}

// vmChanges compares the plan with the VM and returns what has to change.
// Attributes the plan does not give (null or unknown) are left as they are.
func vmChanges(ctx context.Context, plan *vmResourceModel, vm *apiVM, diags *diag.Diagnostics) updateVMRequest {
	var upd updateVMRequest
	if v := plan.Name.ValueString(); v != vm.Name {
		upd.Name = &v
	}
	if v := plan.GuestID.ValueString(); known(plan.GuestID) && v != vm.GuestID {
		upd.GuestID = &v
	}
	if v := int(plan.NumCPUs.ValueInt64()); known(plan.NumCPUs) && v != vm.NumCPUs {
		upd.NumCPUs = &v
	}
	if v := int(plan.CoresPerSocket.ValueInt64()); known(plan.CoresPerSocket) && v != vm.CoresPerSocket {
		upd.CoresPerSocket = &v
	}
	if v := plan.MemoryMB.ValueInt64(); known(plan.MemoryMB) && v != vm.MemoryMB {
		upd.MemoryMB = &v
	}
	if v := plan.Annotation.ValueString(); !plan.Annotation.IsUnknown() && v != vm.Annotation {
		upd.Annotation = &v
	}
	if v := plan.ISOPath.ValueString(); known(plan.ISOPath) && v != vm.CdromISO {
		upd.ISOPath = &v
	}
	if v := plan.Firmware.ValueString(); known(plan.Firmware) && v != firmwareOf(vm) {
		upd.Firmware = &v
	}
	if v := plan.BootDelayMs.ValueInt64(); known(plan.BootDelayMs) && v != vm.BootDelayMs {
		upd.BootDelayMs = &v
	}
	if v := plan.CPUHotAdd.ValueBool(); known(plan.CPUHotAdd) && v != vm.CPUHotAdd {
		upd.CPUHotAdd = &v
	}
	if v := plan.MemoryHotAdd.ValueBool(); known(plan.MemoryHotAdd) && v != vm.MemoryHotAdd {
		upd.MemoryHotAdd = &v
	}
	if v := plan.NestedHV.ValueBool(); known(plan.NestedHV) && v != vm.NestedHV {
		upd.NestedHV = &v
	}
	if v := plan.USBController.ValueString(); known(plan.USBController) && v != vm.USBController {
		upd.USBController = &v
	}
	if v := plan.Template.ValueBool(); known(plan.Template) && v != vm.Template {
		upd.Template = &v
	}

	// Disks: grow the existing ones, append the new ones.
	if known(plan.DiskSizesGB) {
		sizes := toInt64s(ctx, plan.DiskSizesGB, diags)
		if len(sizes) < len(vm.Disks) {
			diags.AddAttributeError(path.Root("disk_sizes_gb"), "removing disks is not supported",
				fmt.Sprintf("The VM has %d disks and disk_sizes_gb lists %d.", len(vm.Disks), len(sizes)))
		}
		for i, cur := range vm.Disks {
			switch {
			case i >= len(sizes):
			case sizes[i] > cur.CapacityGB:
				upd.ResizeDisks = append(upd.ResizeDisks, resizeDisk{Key: cur.Key, SizeGB: sizes[i]})
			case sizes[i] < cur.CapacityGB:
				diags.AddAttributeError(path.Root("disk_sizes_gb"), "shrinking disks is not supported",
					fmt.Sprintf("Disk %d is %d GB and cannot be reduced to %d GB.", i+1, cur.CapacityGB, sizes[i]))
			}
		}
		for i := len(vm.Disks); i < len(sizes); i++ {
			upd.AddDisks = append(upd.AddDisks, apiDiskSpec{SizeGB: sizes[i], Thin: attrBool(plan.DiskThin, true)})
		}
	}

	// Network adapters: the first follows network, the others additional_networks.
	adapter := attrString(plan.AdapterType, "e1000")
	if want := plan.Network.ValueString(); known(plan.Network) && want != "" {
		switch {
		case len(vm.NICs) == 0:
			upd.AddNICs = append(upd.AddNICs, apiNICSpec{Network: want, AdapterType: adapter})
		case vm.NICs[0].Network != want:
			upd.UpdateNICs = append(upd.UpdateNICs, apiNICUpdate{Key: vm.NICs[0].Key, Network: &want})
		}
	}
	if known(plan.AdditionalNetworks) {
		want := toStrings(ctx, plan.AdditionalNetworks, diags)
		var have []apiNIC
		if len(vm.NICs) > 0 {
			have = vm.NICs[1:]
		} else if len(want) > 0 && len(upd.AddNICs) == 0 {
			diags.AddAttributeError(path.Root("additional_networks"), "missing first network adapter",
				"Set network: additional_networks lists the adapters after the first.")
		}
		for i, net := range want {
			switch {
			case i >= len(have):
				upd.AddNICs = append(upd.AddNICs, apiNICSpec{Network: net, AdapterType: adapter})
			case have[i].Network != net:
				upd.UpdateNICs = append(upd.UpdateNICs, apiNICUpdate{Key: have[i].Key, Network: &net})
			}
		}
		for i := len(want); i < len(have); i++ {
			upd.RemoveNICKeys = append(upd.RemoveNICKeys, have[i].Key)
		}
	}
	return upd
}

func firmwareOf(vm *apiVM) string {
	if vm.Firmware == "" {
		return "bios"
	}
	return vm.Firmware
}

// reconcile brings the VM to the planned configuration and power state. The
// VM is powered off first when something has to change, as FreeCenter only
// applies configuration changes to a powered-off VM. It returns the VM as it
// is afterwards.
func (r *vmResource) reconcile(ctx context.Context, hostID, vmID string, plan *vmResourceModel, diags *diag.Diagnostics) (*apiVM, error) {
	vm, err := r.client.GetVM(ctx, hostID, vmID)
	if err != nil {
		return nil, fmt.Errorf("reading VM: %w", err)
	}
	upd := vmChanges(ctx, plan, vm, diags)
	if diags.HasError() {
		return vm, nil
	}
	if !upd.empty() {
		if vm.PowerState != "poweredOff" {
			if err := r.setPower(ctx, hostID, vmID, "off"); err != nil {
				return nil, fmt.Errorf("powering off to apply the changes: %w", err)
			}
		}
		if vm, err = r.client.UpdateVM(ctx, hostID, vmID, upd); err != nil {
			return nil, fmt.Errorf("updating VM: %w", err)
		}
	} else if vm, err = r.client.GetVM(ctx, hostID, vmID); err != nil {
		return nil, fmt.Errorf("reading VM: %w", err)
	}

	// A template stays off whatever the plan says: it cannot be powered on.
	want := attrString(plan.Power, "on")
	if vm.Template {
		want = "off"
	}
	if on := vm.PowerState == "poweredOn"; (want == "on") != on {
		if err := r.setPower(ctx, hostID, vmID, want); err != nil {
			return nil, fmt.Errorf("powering %s: %w", want, err)
		}
	}
	if vm, err = r.client.GetVM(ctx, hostID, vmID); err != nil {
		return nil, fmt.Errorf("reading VM: %w", err)
	}
	return vm, nil
}

// setPower powers the VM on or off and waits for the state to settle.
func (r *vmResource) setPower(ctx context.Context, hostID, vmID, action string) error {
	if err := r.client.SetPower(ctx, hostID, vmID, action); err != nil {
		return err
	}
	want := "poweredOff"
	if action == "on" {
		want = "poweredOn"
	}
	return r.client.WaitForPower(ctx, hostID, vmID, want, powerWait)
}

// setVMState fills m with the VM as FreeCenter reports it. Attributes the API
// does not return (creation options, migration settings) keep their value.
func setVMState(ctx context.Context, m *vmResourceModel, hostID string, vm *apiVM, diags *diag.Diagnostics) {
	m.ID = types.StringValue(hostID + "/" + vm.ID)
	m.HostID = types.StringValue(hostID)
	m.Name = types.StringValue(vm.Name)
	m.GuestID = types.StringValue(vm.GuestID)
	// A VM can touch several datastores (an ISO elsewhere): keep the one the
	// configuration names when the VM is on it.
	if len(vm.Datastores) > 0 && !slices.Contains(vm.Datastores, m.Datastore.ValueString()) {
		m.Datastore = types.StringValue(vm.Datastores[0])
	}
	m.NumCPUs = types.Int64Value(int64(vm.NumCPUs))
	m.CoresPerSocket = types.Int64Value(int64(vm.CoresPerSocket))
	m.MemoryMB = types.Int64Value(vm.MemoryMB)

	sizes := make([]int64, 0, len(vm.Disks))
	for _, d := range vm.Disks {
		sizes = append(sizes, d.CapacityGB)
	}
	disks, d := types.ListValueFrom(ctx, types.Int64Type, sizes)
	diags.Append(d...)
	m.DiskSizesGB = disks
	m.DiskThin = types.BoolValue(attrBool(m.DiskThin, true))

	network, adapter, additional := "", attrString(m.AdapterType, "e1000"), []string{}
	for i, n := range vm.NICs {
		if i == 0 {
			network, adapter = n.Network, n.AdapterType
			continue
		}
		additional = append(additional, n.Network)
	}
	m.Network = types.StringValue(network)
	m.AdditionalNetworks = stringList(ctx, additional, diags)
	m.AdapterType = types.StringValue(adapter)
	m.SCSIController = types.StringValue(attrString(m.SCSIController, "lsilogic-sas"))
	m.Annotation = stringOrNull(vm.Annotation, m.Annotation)
	m.ISOPath = types.StringValue(vm.CdromISO)

	m.Firmware = types.StringValue(firmwareOf(vm))
	m.BootDelayMs = types.Int64Value(vm.BootDelayMs)
	m.CPUHotAdd = types.BoolValue(vm.CPUHotAdd)
	m.MemoryHotAdd = types.BoolValue(vm.MemoryHotAdd)
	m.NestedHV = types.BoolValue(vm.NestedHV)
	m.USBController = types.StringValue(vm.USBController)
	m.Template = types.BoolValue(vm.Template)

	m.Power = types.StringValue("off")
	if vm.PowerState == "poweredOn" {
		m.Power = types.StringValue("on")
	}
	m.IPAddress = types.StringValue(vm.IPAddress)
	m.UUID = types.StringValue(vm.UUID)
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	vm, err := r.client.GetVM(ctx, hostID, vmID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("reading VM failed", err.Error())
		return
	}
	setVMState(ctx, &state, hostID, vm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	ctx = withTaskTarget(ctx, state.Name.ValueString())

	switch target := plan.HostID.ValueString(); {
	case target != hostID:
		newID, ok := r.migrate(ctx, hostID, vmID, &plan, &resp.Diagnostics)
		if !ok {
			return
		}
		// The VM now lives on the target host: record it before going on, so
		// that a failure below does not leave the state pointing at the old id.
		hostID, vmID = target, newID
		state.ID = types.StringValue(hostID + "/" + vmID)
		state.HostID, state.Datastore = plan.HostID, plan.Datastore
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	case !plan.Datastore.Equal(state.Datastore):
		resp.Diagnostics.AddAttributeError(path.Root("datastore"), "the datastore cannot change on the same host",
			"Moving a VM between datastores of one host is not supported: change host_id as well, or replace the VM.")
		return
	}

	vm, err := r.reconcile(ctx, hostID, vmID, &plan, &resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("updating VM failed", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	setVMState(ctx, &plan, hostID, vm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// migrate moves the VM to the host and datastore of the plan and returns its
// id there. The source copy is removed once the VM is registered on the target.
func (r *vmResource) migrate(ctx context.Context, hostID, vmID string, plan *vmResourceModel, diags *diag.Diagnostics) (string, bool) {
	mreq := migrateRequest{
		TargetHostID:    plan.HostID.ValueString(),
		TargetDatastore: plan.Datastore.ValueString(),
		DeleteSource:    true,
		ReclaimThin:     attrBool(plan.MigrateReclaimThin, true),
		Precopy:         attrBool(plan.MigratePrecopy, false),
	}
	check, err := r.client.CheckMigration(ctx, hostID, vmID, mreq)
	if err != nil {
		diags.AddError("checking the migration failed", err.Error())
		return "", false
	}
	if len(check.Blockers) > 0 {
		diags.AddError("the VM cannot be migrated", strings.Join(check.Blockers, "\n"))
		return "", false
	}
	mreq.Mode = attrString(plan.MigrateMode, check.RecommendedMode)
	if check.PowerState != "poweredOn" {
		mreq.Precopy = false // nothing runs: the disks are copied in one go
	}
	if mreq.Mode == "suspend" && !check.CPUCompatible {
		diags.AddAttributeError(path.Root("migrate_mode"), "the hosts have different CPUs",
			fmt.Sprintf("A suspended VM cannot resume on a different CPU (%s -> %s). Use stop_start: the VM is shut down, copied and powered on on the target.",
				check.SourceCPU, check.TargetCPU))
		return "", false
	}
	for _, w := range check.Warnings {
		diags.AddWarning("migration", w)
	}
	job, err := r.client.MigrateVM(ctx, hostID, vmID, mreq)
	if err != nil {
		diags.AddError("migrating VM failed", err.Error())
		return "", false
	}
	jobWarnings(job, diags)
	if job.NewVMID == "" {
		diags.AddError("migrating VM failed", "FreeCenter did not return the id of the VM on the target host.")
		return "", false
	}
	return job.NewVMID, true
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, vmID, err := parseVMID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid state", err.Error())
		return
	}
	ctx = withTaskTarget(ctx, state.Name.ValueString())
	if err := r.client.DeleteVM(ctx, hostID, vmID); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting VM failed", err.Error())
	}
}

func (r *vmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	hostID, _, err := parseVMID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_id"), hostID)...)
}

func diskSpecs(sizes []int64, thin bool) []apiDiskSpec {
	out := make([]apiDiskSpec, 0, len(sizes))
	for _, s := range sizes {
		out = append(out, apiDiskSpec{SizeGB: s, Thin: thin})
	}
	return out
}

func toCloudInitAPI(ctx context.Context, ci *cloudInitModel, diags *diag.Diagnostics) *cloudInitConfig {
	if ci == nil {
		return nil
	}
	cfg := &cloudInitConfig{
		Hostname:          ci.Hostname.ValueString(),
		RootPassword:      ci.RootPassword.ValueString(),
		SSHAuthorizedKeys: toStrings(ctx, ci.SSHAuthorizedKeys, diags),
		Packages:          toStrings(ctx, ci.Packages, diags),
		RunCmd:            toStrings(ctx, ci.RunCmd, diags),
		NetworkConfig:     ci.NetworkConfig.ValueString(),
		UserData:          ci.UserData.ValueString(),
	}
	var users []cloudInitUserModel
	collect(ctx, ci.Users, &users, diags)
	for _, u := range users {
		cfg.Users = append(cfg.Users, cloudInitUser{
			Name:              u.Name.ValueString(),
			Sudo:              u.Sudo.ValueString(),
			Shell:             u.Shell.ValueString(),
			Password:          u.Password.ValueString(),
			SSHAuthorizedKeys: toStrings(ctx, u.SSHAuthorizedKeys, diags),
		})
	}
	var files []cloudInitFileModel
	collect(ctx, ci.WriteFiles, &files, diags)
	for _, f := range files {
		cfg.WriteFiles = append(cfg.WriteFiles, cloudInitFile{
			Path:        f.Path.ValueString(),
			Content:     f.Content.ValueString(),
			Permissions: f.Permissions.ValueString(),
			Owner:       f.Owner.ValueString(),
		})
	}
	return cfg
}

var (
	_ resource.Resource                   = (*vmResource)(nil)
	_ resource.ResourceWithImportState    = (*vmResource)(nil)
	_ resource.ResourceWithValidateConfig = (*vmResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*vmResource)(nil)
)
