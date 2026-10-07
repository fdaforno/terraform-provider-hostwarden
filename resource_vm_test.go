package main

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func int64List(t *testing.T, values ...int64) types.List {
	t.Helper()
	l, diags := types.ListValueFrom(context.Background(), types.Int64Type, values)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return l
}

func stringListValue(t *testing.T, values ...string) types.List {
	t.Helper()
	var diags diag.Diagnostics
	l := stringList(context.Background(), values, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return l
}

// unsetPlan is a plan that only names the VM: everything else is unknown, as
// for attributes the configuration leaves out.
func unsetPlan(name string) vmResourceModel {
	return vmResourceModel{
		Name:               types.StringValue(name),
		GuestID:            types.StringUnknown(),
		NumCPUs:            types.Int64Unknown(),
		CoresPerSocket:     types.Int64Unknown(),
		MemoryMB:           types.Int64Unknown(),
		DiskSizesGB:        types.ListUnknown(types.Int64Type),
		Network:            types.StringUnknown(),
		AdditionalNetworks: types.ListUnknown(types.StringType),
		ISOPath:            types.StringUnknown(),
		Firmware:           types.StringUnknown(),
		BootDelayMs:        types.Int64Unknown(),
		CPUHotAdd:          types.BoolUnknown(),
		MemoryHotAdd:       types.BoolUnknown(),
		NestedHV:           types.BoolUnknown(),
		USBController:      types.StringUnknown(),
		Template:           types.BoolUnknown(),
	}
}

func testVM() *apiVM {
	return &apiVM{
		Name: "vm", NumCPUs: 2, MemoryMB: 1024, GuestID: "ubuntu64Guest", Firmware: "bios",
		Disks: []apiDisk{{Key: 2000, CapacityGB: 10}},
		NICs:  []apiNIC{{Key: 4000, Network: "VM Network"}, {Key: 4001, Network: "dmz"}, {Key: 4002, Network: "lab"}},
	}
}

func TestVMChangesLeavesUnsetAttributesAlone(t *testing.T) {
	var diags diag.Diagnostics
	plan := unsetPlan("vm")
	upd := vmChanges(context.Background(), &plan, testVM(), &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !upd.empty() {
		t.Fatalf("expected no changes, got %+v", upd)
	}
}

func TestVMChangesOptions(t *testing.T) {
	var diags diag.Diagnostics
	plan := unsetPlan("renamed")
	plan.NumCPUs = types.Int64Value(4)
	plan.MemoryMB = types.Int64Value(1024) // unchanged
	plan.Firmware = types.StringValue("efi")
	plan.NestedHV = types.BoolValue(true)
	plan.Template = types.BoolValue(false) // unchanged
	upd := vmChanges(context.Background(), &plan, testVM(), &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if upd.Name == nil || *upd.Name != "renamed" {
		t.Errorf("name change missing: %+v", upd.Name)
	}
	if upd.NumCPUs == nil || *upd.NumCPUs != 4 {
		t.Errorf("cpu change missing: %+v", upd.NumCPUs)
	}
	if upd.Firmware == nil || *upd.Firmware != "efi" || upd.NestedHV == nil || !*upd.NestedHV {
		t.Errorf("firmware / nested virtualization change missing: %+v", upd)
	}
	if upd.MemoryMB != nil || upd.Template != nil {
		t.Errorf("unchanged attributes must not be sent: %+v", upd)
	}
}

func TestVMChangesDisks(t *testing.T) {
	var diags diag.Diagnostics
	plan := unsetPlan("vm")
	plan.DiskSizesGB = int64List(t, 20, 5)
	plan.DiskThin = types.BoolValue(false)
	upd := vmChanges(context.Background(), &plan, testVM(), &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if len(upd.ResizeDisks) != 1 || upd.ResizeDisks[0] != (resizeDisk{Key: 2000, SizeGB: 20}) {
		t.Errorf("resize = %+v, want disk 2000 grown to 20 GB", upd.ResizeDisks)
	}
	if len(upd.AddDisks) != 1 || upd.AddDisks[0] != (apiDiskSpec{SizeGB: 5, Thin: false}) {
		t.Errorf("add = %+v, want one thick disk of 5 GB", upd.AddDisks)
	}

	for name, sizes := range map[string][]int64{"shrink": {5}, "remove": {}} {
		var diags diag.Diagnostics
		plan.DiskSizesGB = int64List(t, sizes...)
		vmChanges(context.Background(), &plan, testVM(), &diags)
		if !diags.HasError() {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestVMChangesNetworks(t *testing.T) {
	var diags diag.Diagnostics
	plan := unsetPlan("vm")
	plan.Network = types.StringValue("prod")
	plan.AdditionalNetworks = stringListValue(t, "dmz2")
	upd := vmChanges(context.Background(), &plan, testVM(), &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if len(upd.UpdateNICs) != 2 || upd.UpdateNICs[0].Key != 4000 || *upd.UpdateNICs[0].Network != "prod" ||
		upd.UpdateNICs[1].Key != 4001 || *upd.UpdateNICs[1].Network != "dmz2" {
		t.Errorf("nic updates = %+v", upd.UpdateNICs)
	}
	if len(upd.RemoveNICKeys) != 1 || upd.RemoveNICKeys[0] != 4002 {
		t.Errorf("nic removals = %v, want [4002]", upd.RemoveNICKeys)
	}
	if len(upd.AddNICs) != 0 {
		t.Errorf("unexpected new nics: %+v", upd.AddNICs)
	}

	// A VM without adapters gets the first one and the additional ones, in order.
	bare := testVM()
	bare.NICs = nil
	plan.AdapterType = types.StringValue("vmxnet3")
	upd = vmChanges(context.Background(), &plan, bare, &diags)
	want := []apiNICSpec{{Network: "prod", AdapterType: "vmxnet3"}, {Network: "dmz2", AdapterType: "vmxnet3"}}
	if len(upd.AddNICs) != 2 || upd.AddNICs[0] != want[0] || upd.AddNICs[1] != want[1] {
		t.Errorf("new nics = %+v, want %+v", upd.AddNICs, want)
	}
}

func TestParseIDs(t *testing.T) {
	if h, v, err := parseVMID("host/42"); err != nil || h != "host" || v != "42" {
		t.Errorf("parseVMID = %q %q %v", h, v, err)
	}
	for _, bad := range []string{"", "host", "host/", "/42", "a/b/c"} {
		if _, _, err := parseVMID(bad); err == nil {
			t.Errorf("parseVMID(%q): expected an error", bad)
		}
	}
	b, err := parseBackupID("host/NVME/web 01/20260102-030405")
	if err != nil || b != (backupID{HostID: "host", Datastore: "NVME", VMName: "web 01", Stamp: "20260102-030405"}) {
		t.Errorf("parseBackupID = %+v %v", b, err)
	}
	if b.String() != "host/NVME/web 01/20260102-030405" {
		t.Errorf("backup id round trip = %q", b.String())
	}
}
