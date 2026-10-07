package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// clientFrom extracts the API client the provider handed to a resource or
// data source. It returns nil before the provider is configured.
func clientFrom(providerData any, diags *diag.Diagnostics) *Client {
	if providerData == nil {
		return nil
	}
	client, ok := providerData.(*Client)
	if !ok {
		diags.AddError("unexpected provider data", fmt.Sprintf("expected *Client, got %T", providerData))
		return nil
	}
	return client
}

// ---- composite ids ----

// parseVMID splits the id of a freecenter_vm: <host_id>/<vm_id>.
func parseVMID(id string) (hostID, vmID string, err error) {
	hostID, vmID, ok := strings.Cut(id, "/")
	if !ok || hostID == "" || vmID == "" || strings.Contains(vmID, "/") {
		return "", "", fmt.Errorf("invalid vm id %q (want <host_id>/<vm_id>)", id)
	}
	return hostID, vmID, nil
}

// splitID splits an id made of exactly n non-empty parts separated by slashes;
// what names the parts in the error message.
func splitID(id string, n int, what string) ([]string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != n {
		return nil, fmt.Errorf("invalid id %q (want %s)", id, what)
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("invalid id %q (want %s)", id, what)
		}
	}
	return parts, nil
}

// backupID identifies a backup: the host and datastore holding it, the VM it
// is of, and its time stamp.
type backupID struct {
	HostID, Datastore, VMName, Stamp string
}

func (b backupID) String() string {
	return strings.Join([]string{b.HostID, b.Datastore, b.VMName, b.Stamp}, "/")
}

func parseBackupID(id string) (backupID, error) {
	p, err := splitID(id, 4, "<host_id>/<datastore>/<vm_name>/<stamp>")
	if err != nil {
		return backupID{}, err
	}
	return backupID{HostID: p[0], Datastore: p[1], VMName: p[2], Stamp: p[3]}, nil
}

// ---- validators ----

type oneOfValidator struct{ values []string }

func oneOf(values ...string) validator.String { return oneOfValidator{values: values} }

func (v oneOfValidator) Description(_ context.Context) string {
	return "value must be one of: " + strings.Join(v.values, ", ")
}

func (v oneOfValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v oneOfValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, ok := range v.values {
		if val == ok {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "invalid value", v.Description(ctx))
}

type int64RangeValidator struct{ min, max int64 }

func int64Between(min, max int64) validator.Int64 { return int64RangeValidator{min: min, max: max} }

func (v int64RangeValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be between %d and %d", v.min, v.max)
}

func (v int64RangeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64RangeValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if n := req.ConfigValue.ValueInt64(); n < v.min || n > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, "invalid value", v.Description(ctx))
	}
}

// ---- attribute helpers ----

// known reports whether the configuration or plan gives a value.
func known(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

func attrString(v types.String, def string) string {
	if !known(v) || v.ValueString() == "" {
		return def
	}
	return v.ValueString()
}

func attrInt64(v types.Int64, def int64) int64 {
	if !known(v) {
		return def
	}
	return v.ValueInt64()
}

func attrBool(v types.Bool, def bool) bool {
	if !known(v) {
		return def
	}
	return v.ValueBool()
}

// stringOrNull returns the API value, or keeps the prior null when both are
// empty (Terraform requires nulls to stay null unless the config changed them).
func stringOrNull(apiVal string, prior types.String) types.String {
	if apiVal != "" {
		return types.StringValue(apiVal)
	}
	if !known(prior) {
		return types.StringNull()
	}
	return types.StringValue("")
}

func toInt64s(ctx context.Context, v attr.Value, diags *diag.Diagnostics) []int64 {
	var out []int64
	collect(ctx, v, &out, diags)
	return out
}

func toStrings(ctx context.Context, v attr.Value, diags *diag.Diagnostics) []string {
	var out []string
	collect(ctx, v, &out, diags)
	return out
}

// collect reads a known list or set into target; null and unknown leave it alone.
func collect(ctx context.Context, v attr.Value, target any, diags *diag.Diagnostics) {
	if !known(v) {
		return
	}
	switch c := v.(type) {
	case types.List:
		diags.Append(c.ElementsAs(ctx, target, false)...)
	case types.Set:
		diags.Append(c.ElementsAs(ctx, target, false)...)
	default:
		diags.AddError("unexpected collection type", fmt.Sprintf("%T is not a list or a set", v))
	}
}

func stringList(ctx context.Context, values []string, diags *diag.Diagnostics) types.List {
	if values == nil {
		values = []string{}
	}
	l, d := types.ListValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return l
}

func stringSet(ctx context.Context, values []string, diags *diag.Diagnostics) types.Set {
	if values == nil {
		values = []string{}
	}
	s, d := types.SetValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return s
}
