package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ---- API models ----

type apiVM struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PowerState     string    `json:"power_state"`
	NumCPUs        int       `json:"num_cpus"`
	CoresPerSocket int       `json:"cores_per_socket"`
	MemoryMB       int64     `json:"memory_mb"`
	GuestID        string    `json:"guest_id"`
	UUID           string    `json:"uuid"`
	Annotation     string    `json:"annotation"`
	IPAddress      string    `json:"ip_address"`
	CdromISO       string    `json:"cdrom_iso"`
	Datastores     []string  `json:"datastores"`
	Disks          []apiDisk `json:"disks"`
	NICs           []apiNIC  `json:"nics"`
	Firmware       string    `json:"firmware"`
	BootDelayMs    int64     `json:"boot_delay_ms"`
	CPUHotAdd      bool      `json:"cpu_hot_add"`
	MemoryHotAdd   bool      `json:"memory_hot_add"`
	NestedHV       bool      `json:"nested_hv"`
	USBController  string    `json:"usb_controller"`
	Template       bool      `json:"template"`
}

type apiNIC struct {
	Key         int32  `json:"key"`
	Network     string `json:"network"`
	AdapterType string `json:"adapter_type"`
	MACAddress  string `json:"mac_address"`
}

type apiDisk struct {
	Key        int32 `json:"key"`
	CapacityGB int64 `json:"capacity_gb"`
}

type cloudInitConfig struct {
	Hostname          string          `json:"hostname,omitempty"`
	RootPassword      string          `json:"root_password,omitempty"`
	SSHAuthorizedKeys []string        `json:"ssh_authorized_keys,omitempty"`
	Users             []cloudInitUser `json:"users,omitempty"`
	Packages          []string        `json:"packages,omitempty"`
	RunCmd            []string        `json:"runcmd,omitempty"`
	WriteFiles        []cloudInitFile `json:"write_files,omitempty"`
	NetworkConfig     string          `json:"network_config,omitempty"`
	UserData          string          `json:"user_data,omitempty"`
}

type cloudInitUser struct {
	Name              string   `json:"name"`
	Sudo              string   `json:"sudo,omitempty"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	Password          string   `json:"password,omitempty"`
}

type cloudInitFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

type createVMRequest struct {
	Name           string           `json:"name"`
	GuestID        string           `json:"guest_id"`
	NumCPUs        int              `json:"num_cpus"`
	CoresPerSocket int              `json:"cores_per_socket,omitempty"`
	MemoryMB       int64            `json:"memory_mb"`
	Datastore      string           `json:"datastore"`
	Disks          []apiDiskSpec    `json:"disks"`
	Network        string           `json:"network,omitempty"`
	AdapterType    string           `json:"adapter_type,omitempty"`
	SCSIController string           `json:"scsi_controller,omitempty"`
	Annotation     string           `json:"annotation,omitempty"`
	ISOPath        string           `json:"iso_path,omitempty"`
	BootFromCDROM  bool             `json:"boot_from_cdrom,omitempty"`
	VNCEnabled     bool             `json:"vnc_enabled,omitempty"`
	VNCPort        int              `json:"vnc_port,omitempty"`
	VNCPassword    string           `json:"vnc_password,omitempty"`
	CloudImage     string           `json:"cloud_image,omitempty"`
	CloudInit      *cloudInitConfig `json:"cloud_init,omitempty"`
	PowerOn        bool             `json:"power_on"`
}

type apiDiskSpec struct {
	SizeGB int64 `json:"size_gb"`
	Thin   bool  `json:"thin"`
}

type apiNICSpec struct {
	Network     string `json:"network"`
	AdapterType string `json:"adapter_type,omitempty"`
}

type apiNICUpdate struct {
	Key     int32   `json:"key"`
	Network *string `json:"network,omitempty"`
}

type resizeDisk struct {
	Key    int32 `json:"key"`
	SizeGB int64 `json:"size_gb"`
}

// updateVMRequest lists the changes to apply; nil fields are left alone.
// Everything in it requires the VM to be powered off.
type updateVMRequest struct {
	NumCPUs        *int           `json:"num_cpus,omitempty"`
	CoresPerSocket *int           `json:"cores_per_socket,omitempty"`
	MemoryMB       *int64         `json:"memory_mb,omitempty"`
	Annotation     *string        `json:"annotation,omitempty"`
	ISOPath        *string        `json:"iso_path,omitempty"`
	ResizeDisks    []resizeDisk   `json:"resize_disks,omitempty"`
	AddDisks       []apiDiskSpec  `json:"add_disks,omitempty"`
	AddNICs        []apiNICSpec   `json:"add_nics,omitempty"`
	UpdateNICs     []apiNICUpdate `json:"update_nics,omitempty"`
	RemoveNICKeys  []int32        `json:"remove_nic_keys,omitempty"`
	Name           *string        `json:"name,omitempty"`
	GuestID        *string        `json:"guest_id,omitempty"`
	Firmware       *string        `json:"firmware,omitempty"`
	BootDelayMs    *int64         `json:"boot_delay_ms,omitempty"`
	CPUHotAdd      *bool          `json:"cpu_hot_add,omitempty"`
	MemoryHotAdd   *bool          `json:"memory_hot_add,omitempty"`
	NestedHV       *bool          `json:"nested_hv,omitempty"`
	USBController  *string        `json:"usb_controller,omitempty"`
	Template       *bool          `json:"template,omitempty"`
}

// empty reports whether the request changes nothing.
func (u *updateVMRequest) empty() bool {
	return u.NumCPUs == nil && u.CoresPerSocket == nil && u.MemoryMB == nil && u.Annotation == nil && u.ISOPath == nil &&
		len(u.ResizeDisks) == 0 && len(u.AddDisks) == 0 && len(u.AddNICs) == 0 && len(u.UpdateNICs) == 0 && len(u.RemoveNICKeys) == 0 &&
		u.Name == nil && u.GuestID == nil && u.Firmware == nil && u.BootDelayMs == nil && u.CPUHotAdd == nil &&
		u.MemoryHotAdd == nil && u.NestedHV == nil && u.USBController == nil && u.Template == nil
}

type migrateRequest struct {
	TargetHostID    string `json:"target_host_id"`
	TargetDatastore string `json:"target_datastore"`
	Mode            string `json:"mode"`
	DeleteSource    bool   `json:"delete_source"`
	ReclaimThin     bool   `json:"reclaim_thin"`
	Precopy         bool   `json:"precopy"`
}

type migrationCheck struct {
	PowerState      string   `json:"power_state"`
	SourceCPU       string   `json:"source_cpu"`
	TargetCPU       string   `json:"target_cpu"`
	CPUCompatible   bool     `json:"cpu_compatible"`
	RecommendedMode string   `json:"recommended_mode"`
	Blockers        []string `json:"blockers"`
	Warnings        []string `json:"warnings"`
}

type cloneRequest struct {
	Name      string `json:"name"`
	Datastore string `json:"datastore"`
	PowerOn   bool   `json:"power_on"`
	Template  bool   `json:"template"`
}

type restoreRequest struct {
	Name      string `json:"name"`
	Datastore string `json:"datastore"`
	PowerOn   bool   `json:"power_on"`
}

// apiJob is a long operation running on the server (migrate, clone, backup, restore).
type apiJob struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	VMName   string   `json:"vm_name"`
	Status   string   `json:"status"` // running | ok | error
	Phase    string   `json:"phase"`
	NewVMID  string   `json:"new_vm_id"`
	Error    string   `json:"error"`
	Warnings []string `json:"warnings"`
}

type apiSnapshot struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createSnapshotRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Memory      bool   `json:"memory"`
	Quiesce     bool   `json:"quiesce"`
}

type apiHostStatus struct {
	Connected       bool   `json:"connected"`
	Version         string `json:"version"`
	Build           string `json:"build"`
	CPUModel        string `json:"cpu_model"`
	CPUCores        int64  `json:"cpu_cores"`
	MemoryTotalMB   int64  `json:"memory_total_mb"`
	MaintenanceMode bool   `json:"maintenance_mode"`
	LicenseName     string `json:"license_name"`
}

type apiHost struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Address    string         `json:"address"`
	Username   string         `json:"username"`
	SSHEnabled bool           `json:"ssh_enabled"`
	SSHPort    int            `json:"ssh_port"`
	Insecure   *bool          `json:"insecure"`
	Status     *apiHostStatus `json:"status"` // only in listings
}

type createHostRequest struct {
	Name       string `json:"name"`
	Address    string `json:"address"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	SSHEnabled bool   `json:"ssh_enabled"`
	SSHPort    int    `json:"ssh_port"`
	Insecure   bool   `json:"insecure"`
}

type apiDatastore struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	FreeB      int64  `json:"free_bytes"`
	CapacityB  int64  `json:"capacity_bytes"`
	Accessible bool   `json:"accessible"`
}

type apiNetwork struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type apiVSwitch struct {
	Name       string   `json:"name"`
	MTU        int64    `json:"mtu"`
	Uplinks    []string `json:"uplinks"`
	PortGroups []string `json:"port_groups"`
}

type apiPortGroup struct {
	Name    string `json:"name"`
	VSwitch string `json:"vswitch"`
	VLAN    int64  `json:"vlan"`
}

type apiHostNetwork struct {
	VSwitches  []apiVSwitch   `json:"vswitches"`
	PortGroups []apiPortGroup `json:"port_groups"`
}

type portGroupRequest struct {
	Name    string `json:"name"`
	VSwitch string `json:"vswitch"`
	VLAN    int64  `json:"vlan"`
}

type vSwitchRequest struct {
	Name    string   `json:"name"`
	Uplinks []string `json:"uplinks"`
}

type apiService struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Running bool   `json:"running"`
	Policy  string `json:"policy"`
}

type serviceRequest struct {
	Action string `json:"action,omitempty"`
	Policy string `json:"policy,omitempty"`
}

type apiHostTime struct {
	NTPEnabled bool     `json:"ntp_enabled"`
	NTPServers []string `json:"ntp_servers"`
}

type apiBackupRequest struct {
	Datastore       string `json:"datastore"`
	TargetHostID    string `json:"target_host_id,omitempty"`
	TargetDatastore string `json:"target_datastore,omitempty"`
	Keep            int    `json:"keep"`
}

type apiBackup struct {
	HostID    string    `json:"host_id"`
	Datastore string    `json:"datastore"`
	VMName    string    `json:"vm_name"`
	Stamp     string    `json:"stamp"`
	Created   time.Time `json:"created"`
	SizeBytes int64     `json:"size_bytes"`
}

type apiSchedule struct {
	ID        string            `json:"id,omitempty"`
	HostID    string            `json:"host_id"`
	VMID      string            `json:"vm_id"`
	Kind      string            `json:"kind"`
	Frequency string            `json:"frequency"`
	Hour      int               `json:"hour"`
	Minute    int               `json:"minute"`
	Weekday   int               `json:"weekday"`
	Timezone  string            `json:"timezone"`
	Keep      int               `json:"keep"`
	Backup    *apiBackupRequest `json:"backup,omitempty"`
	Enabled   bool              `json:"enabled"`
}

type apiUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type userRequest struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Role     string `json:"role,omitempty"`
}

// ---- hosts ----

func (c *Client) ListHosts(ctx context.Context) ([]apiHost, error) {
	var hosts []apiHost
	err := c.do(ctx, http.MethodGet, "/api/v1/hosts", nil, &hosts, shortTimeout)
	return hosts, err
}

// Host returns a registered host with its live status.
func (c *Client) Host(ctx context.Context, id string) (*apiHost, error) {
	hosts, err := c.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range hosts {
		if hosts[i].ID == id {
			return &hosts[i], nil
		}
	}
	return nil, &apiError{Code: http.StatusNotFound, Msg: "host " + id + " not found"}
}

func (c *Client) FindHostByName(ctx context.Context, name string) (*apiHost, error) {
	hosts, err := c.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range hosts {
		if hosts[i].Name == name {
			return &hosts[i], nil
		}
	}
	return nil, &apiError{Code: http.StatusNotFound, Msg: "host " + name + " not found"}
}

func (c *Client) CreateHost(ctx context.Context, req createHostRequest) (*apiHost, error) {
	var host apiHost
	err := c.do(withTaskTarget(ctx, req.Name), http.MethodPost, "/api/v1/hosts", req, &host, shortTimeout)
	return &host, err
}

func (c *Client) UpdateHost(ctx context.Context, id string, req createHostRequest) (*apiHost, error) {
	var host apiHost
	err := c.do(withTaskTarget(ctx, req.Name), http.MethodPut, hostPath(id), req, &host, shortTimeout)
	return &host, err
}

func (c *Client) DeleteHost(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, hostPath(id), nil, nil, shortTimeout)
}

func (c *Client) SetMaintenance(ctx context.Context, hostID string, enabled bool) error {
	body := map[string]bool{"enabled": enabled}
	return c.do(ctx, http.MethodPost, hostPath(hostID, "maintenance"), body, nil, shortTimeout)
}

func (c *Client) ListDatastores(ctx context.Context, hostID string) ([]apiDatastore, error) {
	var ds []apiDatastore
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "datastores"), nil, &ds, shortTimeout)
	return ds, err
}

func (c *Client) ListNetworks(ctx context.Context, hostID string) ([]apiNetwork, error) {
	var nets []apiNetwork
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "networks"), nil, &nets, shortTimeout)
	return nets, err
}

// ---- host configuration ----

func (c *Client) HostNetwork(ctx context.Context, hostID string) (*apiHostNetwork, error) {
	var n apiHostNetwork
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "network"), nil, &n, shortTimeout)
	return &n, err
}

func (c *Client) AddPortGroup(ctx context.Context, hostID string, req portGroupRequest) error {
	return c.do(withTaskTarget(ctx, req.Name), http.MethodPost, hostPath(hostID, "network", "portgroups"), req, nil, shortTimeout)
}

func (c *Client) SetPortGroupVLAN(ctx context.Context, hostID, name string, vlan int64) error {
	req := portGroupRequest{Name: name, VLAN: vlan}
	return c.do(withTaskTarget(ctx, name), http.MethodPut, hostPath(hostID, "network", "portgroups", name), req, nil, shortTimeout)
}

func (c *Client) RemovePortGroup(ctx context.Context, hostID, name string) error {
	return c.do(withTaskTarget(ctx, name), http.MethodDelete, hostPath(hostID, "network", "portgroups", name), nil, nil, shortTimeout)
}

func (c *Client) AddVSwitch(ctx context.Context, hostID string, req vSwitchRequest) error {
	return c.do(withTaskTarget(ctx, req.Name), http.MethodPost, hostPath(hostID, "network", "vswitches"), req, nil, shortTimeout)
}

func (c *Client) RemoveVSwitch(ctx context.Context, hostID, name string) error {
	return c.do(withTaskTarget(ctx, name), http.MethodDelete, hostPath(hostID, "network", "vswitches", name), nil, nil, shortTimeout)
}

func (c *Client) ListServices(ctx context.Context, hostID string) ([]apiService, error) {
	var out []apiService
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "services"), nil, &out, shortTimeout)
	return out, err
}

func (c *Client) SetService(ctx context.Context, hostID, key string, req serviceRequest) error {
	return c.do(withTaskTarget(ctx, key), http.MethodPost, hostPath(hostID, "services", key), req, nil, shortTimeout)
}

func (c *Client) HostTime(ctx context.Context, hostID string) (*apiHostTime, error) {
	var t apiHostTime
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "time"), nil, &t, shortTimeout)
	return &t, err
}

func (c *Client) SetHostTime(ctx context.Context, hostID string, req apiHostTime) error {
	if req.NTPServers == nil {
		req.NTPServers = []string{}
	}
	return c.do(ctx, http.MethodPut, hostPath(hostID, "time"), req, nil, shortTimeout)
}

// ---- virtual machines ----

func (c *Client) ListVMs(ctx context.Context, hostID string) ([]apiVM, error) {
	var vms []apiVM
	err := c.do(ctx, http.MethodGet, hostPath(hostID, "vms"), nil, &vms, shortTimeout)
	return vms, err
}

func (c *Client) CreateVM(ctx context.Context, hostID string, req createVMRequest) (*apiVM, error) {
	var vm apiVM
	err := c.do(withTaskTarget(ctx, req.Name), http.MethodPost, hostPath(hostID, "vms"), req, &vm, longTimeout)
	return &vm, err
}

func (c *Client) GetVM(ctx context.Context, hostID, vmID string) (*apiVM, error) {
	var vm apiVM
	err := c.do(ctx, http.MethodGet, vmPath(hostID, vmID), nil, &vm, shortTimeout)
	return &vm, err
}

func (c *Client) UpdateVM(ctx context.Context, hostID, vmID string, req updateVMRequest) (*apiVM, error) {
	var vm apiVM
	err := c.do(ctx, http.MethodPut, vmPath(hostID, vmID), req, &vm, longTimeout)
	return &vm, err
}

func (c *Client) DeleteVM(ctx context.Context, hostID, vmID string) error {
	return c.do(ctx, http.MethodDelete, vmPath(hostID, vmID)+"?delete_files=true", nil, nil, longTimeout)
}

func (c *Client) SetPower(ctx context.Context, hostID, vmID, action string) error {
	body := map[string]string{"action": action}
	return c.do(ctx, http.MethodPost, vmPath(hostID, vmID, "power"), body, nil, longTimeout)
}

// WaitForPower polls until the VM reaches the desired state or the timeout expires.
func (c *Client) WaitForPower(ctx context.Context, hostID, vmID, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		vm, err := c.GetVM(ctx, hostID, vmID)
		if err != nil {
			return err
		}
		if vm.PowerState == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for VM %s to reach power state %q (current: %q)", vmID, want, vm.PowerState)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) CheckMigration(ctx context.Context, hostID, vmID string, req migrateRequest) (*migrationCheck, error) {
	var check migrationCheck
	err := c.do(ctx, http.MethodPost, vmPath(hostID, vmID, "migrate", "check"), req, &check, shortTimeout)
	return &check, err
}

// runJob posts a request that the server runs in the background and waits for its end.
func (c *Client) runJob(ctx context.Context, path string, body any) (*apiJob, error) {
	var job apiJob
	if err := c.do(ctx, http.MethodPost, path, body, &job, shortTimeout); err != nil {
		return nil, err
	}
	return c.WaitJob(ctx, &job)
}

func (c *Client) MigrateVM(ctx context.Context, hostID, vmID string, req migrateRequest) (*apiJob, error) {
	return c.runJob(ctx, vmPath(hostID, vmID, "migrate"), req)
}

func (c *Client) CloneVM(ctx context.Context, hostID, vmID string, req cloneRequest) (*apiJob, error) {
	return c.runJob(withTaskTarget(ctx, req.Name), vmPath(hostID, vmID, "clone"), req)
}

// ---- snapshots ----

func (c *Client) ListSnapshots(ctx context.Context, hostID, vmID string) ([]apiSnapshot, error) {
	var snaps []apiSnapshot
	err := c.do(ctx, http.MethodGet, vmPath(hostID, vmID, "snapshots"), nil, &snaps, shortTimeout)
	return snaps, err
}

// CreateSnapshot returns the snapshots of the VM after the new one was taken.
func (c *Client) CreateSnapshot(ctx context.Context, hostID, vmID string, req createSnapshotRequest) ([]apiSnapshot, error) {
	var snaps []apiSnapshot
	err := c.do(ctx, http.MethodPost, vmPath(hostID, vmID, "snapshots"), req, &snaps, longTimeout)
	return snaps, err
}

func (c *Client) DeleteSnapshot(ctx context.Context, hostID, vmID, snapID string) error {
	return c.do(ctx, http.MethodDelete, vmPath(hostID, vmID, "snapshots", snapID), nil, nil, longTimeout)
}

// ---- backups and schedules ----

func (c *Client) ListBackups(ctx context.Context) ([]apiBackup, error) {
	var out []apiBackup
	err := c.do(ctx, http.MethodGet, "/api/v1/backups", nil, &out, shortTimeout)
	return out, err
}

func (c *Client) BackupVM(ctx context.Context, hostID, vmID string, req apiBackupRequest) (*apiJob, error) {
	return c.runJob(ctx, vmPath(hostID, vmID, "backup"), req)
}

func backupPath(b backupID, rest ...string) string {
	return hostPath(b.HostID, append([]string{"backups", b.Datastore, b.VMName, b.Stamp}, rest...)...)
}

func (c *Client) DeleteBackup(ctx context.Context, b backupID) error {
	return c.do(withTaskTarget(ctx, b.VMName), http.MethodDelete, backupPath(b), nil, nil, longTimeout)
}

func (c *Client) RestoreBackup(ctx context.Context, b backupID, req restoreRequest) (*apiJob, error) {
	return c.runJob(withTaskTarget(ctx, req.Name), backupPath(b, "restore"), req)
}

func (c *Client) ListSchedules(ctx context.Context) ([]apiSchedule, error) {
	var out []apiSchedule
	err := c.do(ctx, http.MethodGet, "/api/v1/schedules", nil, &out, shortTimeout)
	return out, err
}

// PutSchedule creates the schedule when its ID is empty, and replaces it otherwise.
func (c *Client) PutSchedule(ctx context.Context, sc apiSchedule) (*apiSchedule, error) {
	method, path := http.MethodPost, "/api/v1/schedules"
	if sc.ID != "" {
		method, path = http.MethodPut, "/api/v1/schedules/"+seg(sc.ID)
	}
	var saved apiSchedule
	err := c.do(ctx, method, path, sc, &saved, shortTimeout)
	return &saved, err
}

func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/schedules/"+seg(id), nil, nil, shortTimeout)
}

// ---- users ----

func (c *Client) ListUsers(ctx context.Context) ([]apiUser, error) {
	var out []apiUser
	err := c.do(ctx, http.MethodGet, "/api/v1/users", nil, &out, shortTimeout)
	return out, err
}

func (c *Client) CreateUser(ctx context.Context, req userRequest) (*apiUser, error) {
	var u apiUser
	err := c.do(ctx, http.MethodPost, "/api/v1/users", req, &u, shortTimeout)
	return &u, err
}

func (c *Client) UpdateUser(ctx context.Context, id string, req userRequest) error {
	return c.do(ctx, http.MethodPut, "/api/v1/users/"+seg(id), req, nil, shortTimeout)
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/users/"+seg(id), nil, nil, shortTimeout)
}
