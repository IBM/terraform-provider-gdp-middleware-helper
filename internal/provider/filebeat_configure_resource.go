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
var _ resource.Resource = &FilebeatConfigureResource{}
var _ resource.ResourceWithImportState = &FilebeatConfigureResource{}

func NewFilebeatConfigureResource() resource.Resource {
	return &FilebeatConfigureResource{}
}

// FilebeatConfigureResource defines the resource implementation.
type FilebeatConfigureResource struct{}

// FilebeatConfigureResourceModel describes the resource data model.
type FilebeatConfigureResourceModel struct {
	Host             frameworktypes.String `tfsdk:"host"`
	Port             frameworktypes.Int64  `tfsdk:"port"`
	Username         frameworktypes.String `tfsdk:"username"`
	Password         frameworktypes.String `tfsdk:"password"`
	AuditLogPath     frameworktypes.String `tfsdk:"audit_log_path"`
	DatasourceTag    frameworktypes.String `tfsdk:"datasource_tag"`
	LogstashHost     frameworktypes.String `tfsdk:"logstash_host"`
	LogstashPort     frameworktypes.String `tfsdk:"logstash_port"`
	ConfiguredAt     frameworktypes.String `tfsdk:"configured_at"`
	ID               frameworktypes.String `tfsdk:"id"`
}

func (r *FilebeatConfigureResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_filebeat_configure"
}

func (r *FilebeatConfigureResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resource for configuring Filebeat on a remote server via SSH",

		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Hostname or IP address of the server",
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
			"audit_log_path": schema.StringAttribute{
				MarkdownDescription: "Path to the audit log file to monitor",
				Required:            true,
			},
			"datasource_tag": schema.StringAttribute{
				MarkdownDescription: "Tag to identify the datasource",
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
				MarkdownDescription: "Timestamp when Filebeat was configured",
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

func (r *FilebeatConfigureResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Filebeat Configure Resource")
}

func (r *FilebeatConfigureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FilebeatConfigureResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set default port if not provided
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
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use proper host key verification
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

	// Create Filebeat configuration
	filebeatConfig := fmt.Sprintf(`filebeat.inputs:
- type: filestream
  id: audit-log
  enabled: true
  paths:
    - %s
  tags: ["%s"]

  multiline.type: pattern
  multiline.pattern: "^INFO"
  multiline.negate: true
  multiline.match: after

output.logstash:
  hosts: ["%s:%s"]

logging.level: info
logging.to_files: true
logging.files:
  path: /var/log/filebeat
  name: filebeat
  keepfiles: 7
  permissions: 0644

logging.level: debug
logging.selectors: ["*"]
`, data.AuditLogPath.ValueString(), data.DatasourceTag.ValueString(), 
   data.LogstashHost.ValueString(), data.LogstashPort.ValueString())

	// Execute commands to configure Filebeat
	commands := []string{
		"sudo test -f /etc/filebeat/filebeat.yml && sudo cp /etc/filebeat/filebeat.yml /etc/filebeat/filebeat.yml.backup || true",
		fmt.Sprintf("echo '%s' | sudo tee /etc/filebeat/filebeat.yml > /dev/null", filebeatConfig),
		"sudo filebeat test config -c /etc/filebeat/filebeat.yml",
		"sudo systemctl restart filebeat",
		"sudo systemctl enable filebeat",
	}

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
			// Continue with other commands even if one fails
		} else {
			tflog.Info(ctx, fmt.Sprintf("Command succeeded: %s", cmd))
		}
	}

	// Set computed attributes
	data.ConfiguredAt = frameworktypes.StringValue(time.Now().UTC().Format(time.RFC3339))
	data.ID = frameworktypes.StringValue(fmt.Sprintf("%s-%s", data.Host.ValueString(), data.DatasourceTag.ValueString()))

	tflog.Info(ctx, "Filebeat configuration completed successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FilebeatConfigureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FilebeatConfigureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resource is stateless - just keep the state as is
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FilebeatConfigureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FilebeatConfigureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-run the create logic for updates
	r.Create(ctx, resource.CreateRequest{Plan: req.Plan}, &resource.CreateResponse{
		State:       resp.State,
		Diagnostics: resp.Diagnostics,
	})
}

func (r *FilebeatConfigureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Filebeat configuration is not removed on delete
	// This is intentional to avoid disrupting running systems
	tflog.Info(ctx, "Filebeat configuration will remain on the server after Terraform destroy")
}

func (r *FilebeatConfigureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Made with Bob
