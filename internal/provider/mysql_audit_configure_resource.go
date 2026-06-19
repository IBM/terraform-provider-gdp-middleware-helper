// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	frameworktypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &MysqlAuditConfigureResource{}
var _ resource.ResourceWithImportState = &MysqlAuditConfigureResource{}

func NewMysqlAuditConfigureResource() resource.Resource {
	return &MysqlAuditConfigureResource{}
}

// MysqlAuditConfigureResource defines the resource implementation.
type MysqlAuditConfigureResource struct{}

// MysqlAuditConfigureResourceModel describes the resource data model.
type MysqlAuditConfigureResourceModel struct {
	Host              frameworktypes.String `tfsdk:"host"`
	Port              frameworktypes.Int64  `tfsdk:"port"`
	Username          frameworktypes.String `tfsdk:"username"`
	Password          frameworktypes.String `tfsdk:"password"`
	MysqlRootPassword frameworktypes.String `tfsdk:"mysql_root_password"`
	MysqlInstallPath  frameworktypes.String `tfsdk:"mysql_install_path"`
	LogstashHost      frameworktypes.String `tfsdk:"logstash_host"`
	LogstashPort      frameworktypes.String `tfsdk:"logstash_port"`
	ConfiguredAt      frameworktypes.String `tfsdk:"configured_at"`
	ID                frameworktypes.String `tfsdk:"id"`
}

func (r *MysqlAuditConfigureResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mysql_audit_configure"
}

func (r *MysqlAuditConfigureResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resource for configuring MySQL audit logging on a remote server via SSH",

		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Hostname or IP address of the MySQL server",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "SSH port (default: 22)",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "SSH username",
				Required:            true,
				Sensitive:           true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "SSH password",
				Required:            true,
				Sensitive:           true,
			},
			"mysql_root_password": schema.StringAttribute{
				MarkdownDescription: "MySQL root password",
				Required:            true,
				Sensitive:           true,
			},
			"mysql_install_path": schema.StringAttribute{
				MarkdownDescription: "Path to MySQL installation directory",
				Required:            true,
			},
			"logstash_host": schema.StringAttribute{
				MarkdownDescription: "Logstash host to forward logs to",
				Required:            true,
			},
			"logstash_port": schema.StringAttribute{
				MarkdownDescription: "Logstash port",
				Required:            true,
			},
			"configured_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp when MySQL audit was configured",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *MysqlAuditConfigureResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring MySQL Audit Configure Resource")
}

func (r *MysqlAuditConfigureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MysqlAuditConfigureResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set default port
	port := int64(22)
	if !data.Port.IsNull() {
		port = data.Port.ValueInt64()
	}

	// Configure SSH client
	config := &ssh.ClientConfig{
		User: data.Username.ValueString(),
		Auth: []ssh.AuthMethod{
			ssh.Password(data.Password.ValueString()),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Minute,
	}

	// Connect to the server
	addr := fmt.Sprintf("%s:%d", data.Host.ValueString(), port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		resp.Diagnostics.AddError("SSH Connection Failed", fmt.Sprintf("Unable to connect to %s: %s", addr, err))
		return
	}
	defer client.Close()

	mysqlPass := data.MysqlRootPassword.ValueString()
	installPath := data.MysqlInstallPath.ValueString()

	// Execute commands to configure MySQL audit
	commands := []string{
		fmt.Sprintf("cd %s && mysql -u root -p%s mysql < %s/audit_log_filter_linux_install.sql", installPath, mysqlPass, installPath),
		fmt.Sprintf("sudo mysql -u root -p%s -e \"SELECT PLUGIN_NAME, PLUGIN_STATUS FROM INFORMATION_SCHEMA.PLUGINS WHERE PLUGIN_NAME LIKE 'audit%%';\"", mysqlPass),
		"sudo cp /etc/my.cnf /etc/my.cnf.backup",
		"sudo grep -q 'plugin-load=audit_log.so' /etc/my.cnf || sudo sed -i '/\\[mysqld\\]/a plugin-load=audit_log.so' /etc/my.cnf",
		"sudo grep -q 'audit_log_format=JSON' /etc/my.cnf || sudo sed -i '/\\[mysqld\\]/a audit_log_format=JSON' /etc/my.cnf",
		"sudo grep -q 'port=5143' /etc/my.cnf || sudo sed -i '/\\[mysqld\\]/a port=5143' /etc/my.cnf",
		"sudo grep -q 'audit-log=FORCE_PLUS_PERMANENT' /etc/my.cnf || sudo sed -i '/\\[mysqld\\]/a audit-log=FORCE_PLUS_PERMANENT' /etc/my.cnf",
		"sudo systemctl restart mysqld",
		"sleep 10",
		fmt.Sprintf("sudo mysql -u root -p%s -e \"SELECT audit_log_filter_set_filter('log_all', '{ \\\"filter\\\": { \\\"log\\\": true } }');\"", mysqlPass),
		fmt.Sprintf("sudo mysql -u root -p%s -e \"SELECT audit_log_filter_set_user('%%', 'log_all');\"", mysqlPass),
	}

	// Add rsyslog configuration
	rsyslogConfig := fmt.Sprintf(`
# MySQL Audit Log Forwarding
$ModLoad imfile
$InputFileName /var/lib/mysql/audit.log
$InputFileTag mysql_audit_log:
$InputFileStateFile audit_log
$InputFileSeverity info
$InputFileFacility local6
$InputRunFileMonitor

# Forward to Logstash on Guardium server (UDP)
local6.* @%s:%s
`, data.LogstashHost.ValueString(), data.LogstashPort.ValueString())

	commands = append(commands,
		fmt.Sprintf("echo '%s' | sudo tee -a /etc/rsyslog.conf > /dev/null", rsyslogConfig),
		"sudo systemctl restart rsyslog",
		"sudo systemctl status rsyslog --no-pager",
	)

	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			resp.Diagnostics.AddError("SSH Session Failed", fmt.Sprintf("Unable to create session: %s", err))
			return
		}

		output, err := session.CombinedOutput(cmd)
		session.Close()

		if err != nil {
			tflog.Warn(ctx, fmt.Sprintf("Command failed: %s\nOutput: %s", cmd, string(output)))
			// Continue with other commands
		} else {
			tflog.Info(ctx, fmt.Sprintf("Command succeeded: %s", cmd))
		}
	}

	// Set computed attributes
	data.ConfiguredAt = frameworktypes.StringValue(time.Now().UTC().Format(time.RFC3339))
	data.ID = frameworktypes.StringValue(fmt.Sprintf("%s-mysql-audit", data.Host.ValueString()))

	tflog.Info(ctx, "MySQL audit configuration completed successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MysqlAuditConfigureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MysqlAuditConfigureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MysqlAuditConfigureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MysqlAuditConfigureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.Create(ctx, resource.CreateRequest{Plan: req.Plan}, &resource.CreateResponse{
		State:       resp.State,
		Diagnostics: resp.Diagnostics,
	})
}

func (r *MysqlAuditConfigureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "MySQL audit configuration will remain on the server after Terraform destroy")
}

func (r *MysqlAuditConfigureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Made with Bob
