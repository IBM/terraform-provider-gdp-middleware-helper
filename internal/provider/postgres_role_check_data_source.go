package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jackc/pgx/v5"
)

// Ensure the implementation satisfies the expected interfaces.
var _ datasource.DataSource = &postgresRoleCheckDataSource{}

// NewPostgresRoleCheckDataSource is a helper function to simplify the provider implementation.
func NewPostgresRoleCheckDataSource() datasource.DataSource {
	return &postgresRoleCheckDataSource{}
}

// postgresRoleCheckDataSource is the data source implementation.
type postgresRoleCheckDataSource struct{}

// postgresRoleCheckDataSourceModel maps the data source schema data.
type postgresRoleCheckDataSourceModel struct {
	Host     types.String `tfsdk:"host"`
	Port     types.String `tfsdk:"port"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	DBName   types.String `tfsdk:"db_name"`
	SSLMode  types.String `tfsdk:"ssl_mode"`
	RoleName types.String `tfsdk:"role_name"`
	Exists   types.Bool   `tfsdk:"exists"`
}

// Metadata returns the data source type name.
func (d *postgresRoleCheckDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_role_check"
}

// Schema defines the schema for the data source.
func (d *postgresRoleCheckDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Checks if a role exists in PostgreSQL.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "PostgreSQL server hostname or IP address.",
				Required:    true,
			},
			"port": schema.StringAttribute{
				Description: "PostgreSQL server port.",
				Optional:    true,
				Computed:    true,
			},
			"username": schema.StringAttribute{
				Description: "PostgreSQL username.",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "PostgreSQL password.",
				Required:    true,
				Sensitive:   true,
			},
			"db_name": schema.StringAttribute{
				Description: "PostgreSQL database name.",
				Required:    true,
			},
			"ssl_mode": schema.StringAttribute{
				Description: "PostgreSQL SSL mode (disable, require, verify-ca, verify-full).",
				Optional:    true,
				Computed:    true,
			},
			"role_name": schema.StringAttribute{
				Description: "Name of the role to check for.",
				Required:    true,
			},
			"exists": schema.BoolAttribute{
				Description: "Indicates whether the role exists.",
				Computed:    true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *postgresRoleCheckDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Get current state
	var state postgresRoleCheckDataSourceModel
	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set default values if not provided
	if state.Port.IsNull() || state.Port.ValueString() == "" {
		state.Port = types.StringValue("5432")
	}

	if state.SSLMode.IsNull() || state.SSLMode.ValueString() == "" {
		state.SSLMode = types.StringValue("disable")
	}

	// Build connection string
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		state.Username.ValueString(),
		state.Password.ValueString(),
		state.Host.ValueString(),
		state.Port.ValueString(),
		state.DBName.ValueString(),
		state.SSLMode.ValueString(),
	)

	// Connect to PostgreSQL
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Connect to PostgreSQL",
			fmt.Sprintf("Error connecting to PostgreSQL: %s", err),
		)
		return
	}
	defer conn.Close(ctx)

	// Query to check if the role exists
	query := "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)"

	var exists bool
	err = conn.QueryRow(ctx, query, state.RoleName.ValueString()).Scan(&exists)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Query PostgreSQL",
			fmt.Sprintf("Error querying PostgreSQL: %s", err),
		)
		return
	}

	// Set state
	state.Exists = types.BoolValue(exists)

	// Log the result
	tflog.Info(ctx, fmt.Sprintf("Role %s exists: %t", state.RoleName.ValueString(), exists))

	// Save updated state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
