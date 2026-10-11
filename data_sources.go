package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// resolveHost returns the host id given directly or through the host name.
func resolveHost(ctx context.Context, c *Client, hostID, hostName types.String, diags *diag.Diagnostics) (string, bool) {
	if id := hostID.ValueString(); id != "" {
		return id, true
	}
	name := hostName.ValueString()
	if name == "" {
		diags.AddError("missing host", "Set host_id or host_name.")
		return "", false
	}
	host, err := c.FindHostByName(ctx, name)
	if err != nil {
		diags.AddError("host lookup failed", err.Error())
		return "", false
	}
	return host.ID, true
}

// ---- data source: hostwarden_host (by name) ----

type hostDataSource struct{ client *Client }

func NewHostDataSource() datasource.DataSource { return &hostDataSource{} }

type hostDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Address         types.String `tfsdk:"address"`
	SSHEnabled      types.Bool   `tfsdk:"ssh_enabled"`
	Connected       types.Bool   `tfsdk:"connected"`
	Version         types.String `tfsdk:"version"`
	Build           types.String `tfsdk:"build"`
	CPUModel        types.String `tfsdk:"cpu_model"`
	CPUCores        types.Int64  `tfsdk:"cpu_cores"`
	MemoryTotalMB   types.Int64  `tfsdk:"memory_total_mb"`
	MaintenanceMode types.Bool   `tfsdk:"maintenance_mode"`
	LicenseName     types.String `tfsdk:"license_name"`
}

func (d *hostDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_host"
}

func (d *hostDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a registered Hostwarden host by name, with its live status.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true},
			"name":             schema.StringAttribute{Required: true},
			"address":          schema.StringAttribute{Computed: true},
			"ssh_enabled":      schema.BoolAttribute{Computed: true},
			"connected":        schema.BoolAttribute{Computed: true, Description: "Whether Hostwarden reaches the host. The attributes below are empty when it does not."},
			"version":          schema.StringAttribute{Computed: true, Description: "ESXi version."},
			"build":            schema.StringAttribute{Computed: true, Description: "ESXi build number."},
			"cpu_model":        schema.StringAttribute{Computed: true, Description: "CPU model: hosts with different CPUs cannot exchange suspended VMs."},
			"cpu_cores":        schema.Int64Attribute{Computed: true},
			"memory_total_mb":  schema.Int64Attribute{Computed: true},
			"maintenance_mode": schema.BoolAttribute{Computed: true},
			"license_name":     schema.StringAttribute{Computed: true},
		},
	}
}

func (d *hostDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *hostDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data hostDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	host, err := d.client.FindHostByName(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("host lookup failed", err.Error())
		return
	}
	st := apiHostStatus{}
	if host.Status != nil {
		st = *host.Status
	}
	data.ID = types.StringValue(host.ID)
	data.Address = types.StringValue(host.Address)
	data.SSHEnabled = types.BoolValue(host.SSHEnabled)
	data.Connected = types.BoolValue(st.Connected)
	data.Version = types.StringValue(st.Version)
	data.Build = types.StringValue(st.Build)
	data.CPUModel = types.StringValue(st.CPUModel)
	data.CPUCores = types.Int64Value(st.CPUCores)
	data.MemoryTotalMB = types.Int64Value(st.MemoryTotalMB)
	data.MaintenanceMode = types.BoolValue(st.MaintenanceMode)
	data.LicenseName = types.StringValue(st.LicenseName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---- data source: hostwarden_hosts (all) ----

type hostsDataSource struct{ client *Client }

func NewHostsDataSource() datasource.DataSource { return &hostsDataSource{} }

type hostsDataSourceModel struct {
	Hosts []hostEntry `tfsdk:"hosts"`
}

type hostEntry struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Address   types.String `tfsdk:"address"`
	Connected types.Bool   `tfsdk:"connected"`
}

func (d *hostsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_hosts"
}

func (d *hostsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All hosts registered in Hostwarden.",
		Attributes: map[string]schema.Attribute{
			"hosts": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":        schema.StringAttribute{Computed: true},
						"name":      schema.StringAttribute{Computed: true},
						"address":   schema.StringAttribute{Computed: true},
						"connected": schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *hostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *hostsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	hosts, err := d.client.ListHosts(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing hosts failed", err.Error())
		return
	}
	data := hostsDataSourceModel{Hosts: make([]hostEntry, 0, len(hosts))}
	for i := range hosts {
		data.Hosts = append(data.Hosts, hostEntry{
			ID:        types.StringValue(hosts[i].ID),
			Name:      types.StringValue(hosts[i].Name),
			Address:   types.StringValue(hosts[i].Address),
			Connected: types.BoolValue(hosts[i].Status != nil && hosts[i].Status.Connected),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---- data source: hostwarden_datastores ----

type datastoresDataSource struct{ client *Client }

func NewDatastoresDataSource() datasource.DataSource { return &datastoresDataSource{} }

type datastoresDataSourceModel struct {
	HostID     types.String     `tfsdk:"host_id"`
	HostName   types.String     `tfsdk:"host_name"`
	Datastores []datastoreEntry `tfsdk:"datastores"`
}

type datastoreEntry struct {
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	FreeBytes     types.Int64  `tfsdk:"free_bytes"`
	CapacityBytes types.Int64  `tfsdk:"capacity_bytes"`
	Accessible    types.Bool   `tfsdk:"accessible"`
}

func (d *datastoresDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_datastores"
}

func (d *datastoresDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Datastores of a Hostwarden host (by host_id or host_name).",
		Attributes: map[string]schema.Attribute{
			"host_id":   schema.StringAttribute{Optional: true, Computed: true},
			"host_name": schema.StringAttribute{Optional: true},
			"datastores": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":           schema.StringAttribute{Computed: true},
						"type":           schema.StringAttribute{Computed: true},
						"free_bytes":     schema.Int64Attribute{Computed: true},
						"capacity_bytes": schema.Int64Attribute{Computed: true},
						"accessible":     schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *datastoresDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *datastoresDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data datastoresDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, ok := resolveHost(ctx, d.client, data.HostID, data.HostName, &resp.Diagnostics)
	if !ok {
		return
	}
	dss, err := d.client.ListDatastores(ctx, hostID)
	if err != nil {
		resp.Diagnostics.AddError("listing datastores failed", err.Error())
		return
	}
	data.HostID = types.StringValue(hostID)
	data.Datastores = make([]datastoreEntry, 0, len(dss))
	for i := range dss {
		data.Datastores = append(data.Datastores, datastoreEntry{
			Name:          types.StringValue(dss[i].Name),
			Type:          types.StringValue(dss[i].Type),
			FreeBytes:     types.Int64Value(dss[i].FreeB),
			CapacityBytes: types.Int64Value(dss[i].CapacityB),
			Accessible:    types.BoolValue(dss[i].Accessible),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---- data source: hostwarden_networks ----

type networksDataSource struct{ client *Client }

func NewNetworksDataSource() datasource.DataSource { return &networksDataSource{} }

type networksDataSourceModel struct {
	HostID     types.String     `tfsdk:"host_id"`
	HostName   types.String     `tfsdk:"host_name"`
	Networks   types.List       `tfsdk:"networks"`
	PortGroups []portGroupEntry `tfsdk:"port_groups"`
	VSwitches  []vSwitchEntry   `tfsdk:"vswitches"`
}

type portGroupEntry struct {
	Name    types.String `tfsdk:"name"`
	VSwitch types.String `tfsdk:"vswitch"`
	VLAN    types.Int64  `tfsdk:"vlan"`
}

type vSwitchEntry struct {
	Name    types.String `tfsdk:"name"`
	MTU     types.Int64  `tfsdk:"mtu"`
	Uplinks types.List   `tfsdk:"uplinks"`
}

func (d *networksDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_networks"
}

func (d *networksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Networking of a Hostwarden host (by host_id or host_name): the networks virtual machines can connect to, " +
			"and the standard virtual switches and port groups behind them.",
		Attributes: map[string]schema.Attribute{
			"host_id":   schema.StringAttribute{Optional: true, Computed: true},
			"host_name": schema.StringAttribute{Optional: true},
			"networks": schema.ListAttribute{
				Description: "Names of the networks available to virtual machines.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"port_groups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":    schema.StringAttribute{Computed: true},
						"vswitch": schema.StringAttribute{Computed: true},
						"vlan":    schema.Int64Attribute{Computed: true},
					},
				},
			},
			"vswitches": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":    schema.StringAttribute{Computed: true},
						"mtu":     schema.Int64Attribute{Computed: true},
						"uplinks": schema.ListAttribute{ElementType: types.StringType, Computed: true},
					},
				},
			},
		},
	}
}

func (d *networksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *networksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data networksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, ok := resolveHost(ctx, d.client, data.HostID, data.HostName, &resp.Diagnostics)
	if !ok {
		return
	}
	nets, err := d.client.ListNetworks(ctx, hostID)
	if err != nil {
		resp.Diagnostics.AddError("listing networks failed", err.Error())
		return
	}
	detail, err := d.client.HostNetwork(ctx, hostID)
	if err != nil {
		resp.Diagnostics.AddError("reading host network failed", err.Error())
		return
	}
	names := make([]string, 0, len(nets))
	for _, n := range nets {
		names = append(names, n.Name)
	}
	data.HostID = types.StringValue(hostID)
	data.Networks = stringList(ctx, names, &resp.Diagnostics)
	data.PortGroups = make([]portGroupEntry, 0, len(detail.PortGroups))
	for _, pg := range detail.PortGroups {
		data.PortGroups = append(data.PortGroups, portGroupEntry{
			Name:    types.StringValue(pg.Name),
			VSwitch: types.StringValue(pg.VSwitch),
			VLAN:    types.Int64Value(pg.VLAN),
		})
	}
	data.VSwitches = make([]vSwitchEntry, 0, len(detail.VSwitches))
	for _, sw := range detail.VSwitches {
		data.VSwitches = append(data.VSwitches, vSwitchEntry{
			Name:    types.StringValue(sw.Name),
			MTU:     types.Int64Value(sw.MTU),
			Uplinks: stringList(ctx, sw.Uplinks, &resp.Diagnostics),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---- data source: hostwarden_vm (by host and name) ----

type vmDataSource struct{ client *Client }

func NewVMDataSource() datasource.DataSource { return &vmDataSource{} }

type vmDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	HostID     types.String `tfsdk:"host_id"`
	HostName   types.String `tfsdk:"host_name"`
	Name       types.String `tfsdk:"name"`
	VMID       types.String `tfsdk:"vm_id"`
	PowerState types.String `tfsdk:"power_state"`
	NumCPUs    types.Int64  `tfsdk:"num_cpus"`
	MemoryMB   types.Int64  `tfsdk:"memory_mb"`
	GuestID    types.String `tfsdk:"guest_id"`
	Firmware   types.String `tfsdk:"firmware"`
	Template   types.Bool   `tfsdk:"template"`
	IPAddress  types.String `tfsdk:"ip_address"`
	UUID       types.String `tfsdk:"uuid"`
	Datastores types.List   `tfsdk:"datastores"`
	Networks   types.List   `tfsdk:"networks"`
}

func (d *vmDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_vm"
}

func (d *vmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a virtual machine or template by name on a host (by host_id or host_name), " +
			"e.g. a template to clone with clone_from.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, Description: "<host_id>/<vm_id>: usable as clone_from and vm_id."},
			"host_id":     schema.StringAttribute{Optional: true, Computed: true},
			"host_name":   schema.StringAttribute{Optional: true},
			"name":        schema.StringAttribute{Required: true},
			"vm_id":       schema.StringAttribute{Computed: true, Description: "Id of the VM on its host."},
			"power_state": schema.StringAttribute{Computed: true, Description: "poweredOn, poweredOff or suspended."},
			"num_cpus":    schema.Int64Attribute{Computed: true},
			"memory_mb":   schema.Int64Attribute{Computed: true},
			"guest_id":    schema.StringAttribute{Computed: true},
			"firmware":    schema.StringAttribute{Computed: true},
			"template":    schema.BoolAttribute{Computed: true},
			"ip_address":  schema.StringAttribute{Computed: true},
			"uuid":        schema.StringAttribute{Computed: true},
			"datastores":  schema.ListAttribute{ElementType: types.StringType, Computed: true},
			"networks":    schema.ListAttribute{ElementType: types.StringType, Computed: true, Description: "Port group of each network adapter, in order."},
		},
	}
}

func (d *vmDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *vmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data vmDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostID, ok := resolveHost(ctx, d.client, data.HostID, data.HostName, &resp.Diagnostics)
	if !ok {
		return
	}
	vms, err := d.client.ListVMs(ctx, hostID)
	if err != nil {
		resp.Diagnostics.AddError("listing VMs failed", err.Error())
		return
	}
	name, vmID := data.Name.ValueString(), ""
	for i := range vms {
		if vms[i].Name != name {
			continue
		}
		if vmID != "" {
			resp.Diagnostics.AddError("ambiguous VM name", fmt.Sprintf("More than one VM is named %q on the host.", name))
			return
		}
		vmID = vms[i].ID
	}
	if vmID == "" {
		resp.Diagnostics.AddError("VM not found", fmt.Sprintf("No VM is named %q on the host.", name))
		return
	}
	// The listing is a summary: devices come with the VM itself.
	vm, err := d.client.GetVM(ctx, hostID, vmID)
	if err != nil {
		resp.Diagnostics.AddError("reading VM failed", err.Error())
		return
	}
	networks := make([]string, 0, len(vm.NICs))
	for _, n := range vm.NICs {
		networks = append(networks, n.Network)
	}
	data.ID = types.StringValue(hostID + "/" + vm.ID)
	data.HostID = types.StringValue(hostID)
	data.VMID = types.StringValue(vm.ID)
	data.PowerState = types.StringValue(vm.PowerState)
	data.NumCPUs = types.Int64Value(int64(vm.NumCPUs))
	data.MemoryMB = types.Int64Value(vm.MemoryMB)
	data.GuestID = types.StringValue(vm.GuestID)
	data.Firmware = types.StringValue(firmwareOf(vm))
	data.Template = types.BoolValue(vm.Template)
	data.IPAddress = types.StringValue(vm.IPAddress)
	data.UUID = types.StringValue(vm.UUID)
	data.Datastores = stringList(ctx, vm.Datastores, &resp.Diagnostics)
	data.Networks = stringList(ctx, networks, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---- data source: hostwarden_backups ----

type backupsDataSource struct{ client *Client }

func NewBackupsDataSource() datasource.DataSource { return &backupsDataSource{} }

type backupsDataSourceModel struct {
	VMName  types.String  `tfsdk:"vm_name"`
	HostID  types.String  `tfsdk:"host_id"`
	Backups []backupEntry `tfsdk:"backups"`
}

type backupEntry struct {
	ID        types.String `tfsdk:"id"`
	HostID    types.String `tfsdk:"host_id"`
	Datastore types.String `tfsdk:"datastore"`
	VMName    types.String `tfsdk:"vm_name"`
	Stamp     types.String `tfsdk:"stamp"`
	Created   types.String `tfsdk:"created"`
	SizeBytes types.Int64  `tfsdk:"size_bytes"`
}

func (d *backupsDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "hostwarden_backups"
}

func (d *backupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Backups found on the connected hosts, newest first, optionally of one VM or on one host.",
		Attributes: map[string]schema.Attribute{
			"vm_name": schema.StringAttribute{Optional: true, Description: "Only the backups of the VM with this name."},
			"host_id": schema.StringAttribute{Optional: true, Description: "Only the backups held by this host."},
			"backups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, Description: "Usable as restore_from of hostwarden_vm."},
						"host_id":    schema.StringAttribute{Computed: true},
						"datastore":  schema.StringAttribute{Computed: true},
						"vm_name":    schema.StringAttribute{Computed: true},
						"stamp":      schema.StringAttribute{Computed: true},
						"created":    schema.StringAttribute{Computed: true},
						"size_bytes": schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *backupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *backupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data backupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := d.client.ListBackups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing backups failed", err.Error())
		return
	}
	vmName, hostID := data.VMName.ValueString(), data.HostID.ValueString()
	data.Backups = []backupEntry{}
	for _, b := range list {
		if (vmName != "" && b.VMName != vmName) || (hostID != "" && b.HostID != hostID) {
			continue
		}
		id := backupID{HostID: b.HostID, Datastore: b.Datastore, VMName: b.VMName, Stamp: b.Stamp}
		data.Backups = append(data.Backups, backupEntry{
			ID:        types.StringValue(id.String()),
			HostID:    types.StringValue(b.HostID),
			Datastore: types.StringValue(b.Datastore),
			VMName:    types.StringValue(b.VMName),
			Stamp:     types.StringValue(b.Stamp),
			Created:   types.StringValue(b.Created.UTC().Format(time.RFC3339)),
			SizeBytes: types.Int64Value(b.SizeBytes),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
