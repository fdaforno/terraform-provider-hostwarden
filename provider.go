package main

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NewProvider returns a fresh provider instance.
func NewProvider(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hostwardenProvider{version: version}
	}
}

type hostwardenProvider struct {
	version string
}

type providerConfig struct {
	Address  types.String `tfsdk:"address"`
	Token    types.String `tfsdk:"token"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func (p *hostwardenProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hostwarden"
	resp.Version = p.version
}

func (p *hostwardenProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage standalone ESXi hosts and their virtual machines through a Hostwarden server. " +
			"Authenticate with a token, or with a user name and password. Changes require an administrator.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Description: "Base URL of the Hostwarden server (e.g. http://hostwarden:8080). Env: HOSTWARDEN_ADDRESS.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "API bearer token, from POST /api/v1/auth/login. A token expires after 7 days without use. Env: HOSTWARDEN_TOKEN.",
				Optional:    true,
				Sensitive:   true,
			},
			"username": schema.StringAttribute{
				Description: "User name, as an alternative to token: the provider opens the session itself. Env: HOSTWARDEN_USERNAME.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password of username. Env: HOSTWARDEN_PASSWORD.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// setting returns the configured value, or the environment variable when the
// configuration leaves it out.
func setting(v types.String, env string) string {
	if known(v) && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func (p *hostwardenProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerConfig
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	address := setting(cfg.Address, "HOSTWARDEN_ADDRESS")
	token := setting(cfg.Token, "HOSTWARDEN_TOKEN")
	username := setting(cfg.Username, "HOSTWARDEN_USERNAME")
	password := setting(cfg.Password, "HOSTWARDEN_PASSWORD")

	if address == "" {
		resp.Diagnostics.AddAttributeError(path.Root("address"), "missing Hostwarden address",
			"Set the address attribute or the HOSTWARDEN_ADDRESS environment variable.")
	}
	switch {
	case token != "":
	case username == "" && password == "":
		resp.Diagnostics.AddAttributeError(path.Root("token"), "missing Hostwarden credentials",
			"Set token (HOSTWARDEN_TOKEN), or username and password (HOSTWARDEN_USERNAME, HOSTWARDEN_PASSWORD).")
	case username == "" || password == "":
		resp.Diagnostics.AddAttributeError(path.Root("username"), "incomplete Hostwarden credentials",
			"Set both username and password.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client := NewClient(address, token, username, password)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *hostwardenProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewHostResource,
		NewHostServiceResource,
		NewHostNTPResource,
		NewPortGroupResource,
		NewVSwitchResource,
		NewVMResource,
		NewSnapshotResource,
		NewBackupResource,
		NewScheduleResource,
		NewUserResource,
	}
}

func (p *hostwardenProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewHostDataSource,
		NewHostsDataSource,
		NewDatastoresDataSource,
		NewNetworksDataSource,
		NewVMDataSource,
		NewBackupsDataSource,
	}
}
