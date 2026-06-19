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
var _ resource.Resource = &RsyslogConfigureResource{}
var _ resource.ResourceWithImportState = &RsyslogConfigureResource{}

func NewRsyslogConfigureResource() resource.Resource {
	return &RsyslogConfigureResource{}
}

// RsyslogConfigureResource defines the resource implementation.
type RsyslogConfigureResource struct{}

// RsyslogConfigureResourceModel describes the resource data model.
type RsyslogConfigureResourceModel struct {
	Host          frameworktypes.String `tfsdk:"host"`
	Port          frameworktypes.Int64  `tfsdk:"port"`
	Username      frameworktypes.String `tfsdk:"username"`
	Password      frameworktypes.String `tfsdk:"password"`
	LogFilePath   frameworktypes.String `tfsdk:"log_file_path"`
	LogTag        frameworktypes.String `tfsdk:"log_tag"`
	LogstashHost  frameworktypes.String `tfsdk:"logstash_host"`
	LogstashPort  frameworktypes.String `tfsdk:"logstash_port"`
	Facility      frameworktypes.String `tfsdk:"facility"`
	ConfiguredAt  frameworktypes.String `tfsdk:"configured_at"`
	ID            frameworktypes.String `tfsdk:"id"`
}

func (r *RsyslogConfigureResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rsyslog_configure"
}

func (r *RsyslogConfigureResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resource for configuring rsyslog on a remote server via SSH",

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
			"log_file_path": schema.StringAttribute{
				MarkdownDescription: "Path to the log file to monitor",
				Required:            true,
			},
			"log_tag": schema.StringAttribute{
				MarkdownDescription: "Tag to identify the log source",
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
			"facility": schema.StringAttribute{
				MarkdownDescription: "Syslog facility (default: local6)",
				Optional:            true,
			},
			"configured_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp when rsyslog was configured",
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

func (r *RsyslogConfigureResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Rsyslog Configure Resource")
}

func (r *RsyslogConfigureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RsyslogConfigureResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set defaults
	port := int64(22)
	if !data.Port.IsNull() {
		port = data.Port.ValueInt64()
	}

	facility := "local6"
	if !data.Facility.IsNull() {
		facility = data.Facility.ValueString()
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

	// Create rsyslog configuration
	rsyslogConfig := fmt.Sprintf(`
# %s Log Forwarding
$ModLoad imfile
$InputFileName %s
$InputFileTag %s:
$InputFileStateFile %s_state
$InputFileSeverity info
$InputFileFacility %s
$InputRunFileMonitor

# Forward to Logstash on Guardium server (UDP)
%s.* @%s:%s
`, data.LogTag.ValueString(), data.LogFilePath.ValueString(), data.LogTag.ValueString(),
   data.LogTag.ValueString(), facility, facility, data.LogstashHost.ValueString(), data.LogstashPort.ValueString())

	// Execute commands to configure rsyslog
	commands := []string{
		fmt.Sprintf("echo '%s' | sudo tee -a /etc/rsyslog.conf > /dev/null", rsyslogConfig),
		"sudo systemctl restart rsyslog",
		"sudo systemctl enable rsyslog",
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
		} else {
			tflog.Info(ctx, fmt.Sprintf("Command succeeded: %s", cmd))
		}
	}

	// Set computed attributes
	data.ConfiguredAt = frameworktypes.StringValue(time.Now().UTC().Format(time.RFC3339))
	data.ID = frameworktypes.StringValue(fmt.Sprintf("%s-%s", data.Host.ValueString(), data.LogTag.ValueString()))

	tflog.Info(ctx, "Rsyslog configuration completed successfully")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RsyslogConfigureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RsyslogConfigureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RsyslogConfigureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RsyslogConfigureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.Create(ctx, resource.CreateRequest{Plan: req.Plan}, &resource.CreateResponse{
		State:       resp.State,
		Diagnostics: resp.Diagnostics,
	})
}

func (r *RsyslogConfigureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "Rsyslog configuration will remain on the server after Terraform destroy")
}

func (r *RsyslogConfigureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Made with Bob
