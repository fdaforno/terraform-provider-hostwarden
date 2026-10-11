package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type userResource struct{ client *Client }

func NewUserResource() resource.Resource { return &userResource{} }

type userModel struct {
	ID       types.String `tfsdk:"id"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Role     types.String `tfsdk:"role"`
}

func (r *userResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "hostwarden_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Hostwarden user. Managing users requires the provider to authenticate as an administrator.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				Description:   "User name: letters, digits and . _ @ - (max 64).",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"password": schema.StringAttribute{
				Description: "Password, at least 8 characters. Changing it signs the user out everywhere.",
				Required:    true,
				Sensitive:   true,
			},
			"role": schema.StringAttribute{
				Description: "admin (everything) or readonly (can look, cannot change anything nor open consoles).",
				Required:    true,
				Validators:  []validator.String{oneOf("admin", "readonly")},
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.CreateUser(ctx, userRequest{
		Username: plan.Username.ValueString(),
		Password: plan.Password.ValueString(),
		Role:     plan.Role.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("creating user failed", err.Error())
		return
	}
	plan.ID = types.StringValue(user.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	users, err := r.client.ListUsers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("listing users failed", err.Error())
		return
	}
	for _, u := range users {
		if u.ID == state.ID.ValueString() {
			state.Username = types.StringValue(u.Username)
			state.Role = types.StringValue(u.Role)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only what changed is sent: resetting a password ends the user's sessions.
	var upd userRequest
	if !plan.Role.Equal(state.Role) {
		upd.Role = plan.Role.ValueString()
	}
	if !plan.Password.Equal(state.Password) {
		upd.Password = plan.Password.ValueString()
	}
	if upd != (userRequest{}) {
		if err := r.client.UpdateUser(ctx, state.ID.ValueString(), upd); err != nil {
			resp.Diagnostics.AddError("updating user failed", err.Error())
			return
		}
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUser(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("deleting user failed", err.Error())
	}
}

var _ resource.Resource = (*userResource)(nil)
