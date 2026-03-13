// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	frameworktypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &OpenSearchModifyResource{}
var _ resource.ResourceWithImportState = &OpenSearchModifyResource{}

func NewOpenSearchModifyResource() resource.Resource {
	return &OpenSearchModifyResource{}
}

// OpenSearchModifyResource defines the resource implementation.
type OpenSearchModifyResource struct {
	client *opensearch.Client
}

// OpenSearchModifyResourceModel describes the resource data model.
type OpenSearchModifyResourceModel struct {
	DomainName                       frameworktypes.String `tfsdk:"domain_name"`
	Region                           frameworktypes.String `tfsdk:"region"`
	AuditLogsEnabled                 frameworktypes.Bool   `tfsdk:"audit_logs_enabled"`
	AuditLogsGroupArn                frameworktypes.String `tfsdk:"audit_logs_group_arn"`
	ProfilerLogsEnabled              frameworktypes.Bool   `tfsdk:"profiler_logs_enabled"`
	ProfilerLogsGroupArn             frameworktypes.String `tfsdk:"profiler_logs_group_arn"`
	MasterUsername                   frameworktypes.String `tfsdk:"master_username"`
	MasterPassword                   frameworktypes.String `tfsdk:"master_password"`
	EnableSecurityPluginAuditing     frameworktypes.Bool   `tfsdk:"enable_security_plugin_auditing"`
	AuditRestDisabledCategories      frameworktypes.List   `tfsdk:"audit_rest_disabled_categories"`
	AuditDisabledTransportCategories frameworktypes.List   `tfsdk:"audit_disabled_transport_categories"`
	LastModifiedTime                 frameworktypes.String `tfsdk:"last_modified_time"`
	ID                               frameworktypes.String `tfsdk:"id"`
}

func (r *OpenSearchModifyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_opensearch_modify"
}

func (r *OpenSearchModifyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for modifying an AWS OpenSearch domain configuration to enable audit logging",

		Attributes: map[string]schema.Attribute{
			"domain_name": schema.StringAttribute{
				MarkdownDescription: "The name of the OpenSearch domain to modify",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the OpenSearch domain is located",
				Optional:            true,
			},
			"audit_logs_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether to enable audit logs",
				Required:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"audit_logs_group_arn": schema.StringAttribute{
				MarkdownDescription: "CloudWatch Logs group ARN for audit logs",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"profiler_logs_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether to enable profiler logs (INDEX_SLOW_LOGS)",
				Optional:            true,
			},
			"profiler_logs_group_arn": schema.StringAttribute{
				MarkdownDescription: "CloudWatch Logs group ARN for profiler logs",
				Optional:            true,
			},
			"master_username": schema.StringAttribute{
				MarkdownDescription: "Master username for OpenSearch domain (required to enable security plugin auditing)",
				Optional:            true,
				Sensitive:           true,
			},
			"master_password": schema.StringAttribute{
				MarkdownDescription: "Master password for OpenSearch domain (required to enable security plugin auditing)",
				Optional:            true,
				Sensitive:           true,
			},
			"enable_security_plugin_auditing": schema.BoolAttribute{
				MarkdownDescription: "Whether to enable audit logging in the OpenSearch security plugin (requires master credentials)",
				Optional:            true,
			},
			"audit_rest_disabled_categories": schema.ListAttribute{
				MarkdownDescription: "List of REST audit categories to disable (all categories enabled by default)",
				Optional:            true,
				ElementType:         frameworktypes.StringType,
			},
			"audit_disabled_transport_categories": schema.ListAttribute{
				MarkdownDescription: "List of Transport audit categories to disable (all categories enabled by default)",
				Optional:            true,
				ElementType:         frameworktypes.StringType,
			},
			"last_modified_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last modification operation",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OpenSearchModifyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring OpenSearch modify resource")

	// If provider is not configured, return
	if req.ProviderData == nil {
		return
	}

	// Create AWS config and OpenSearch client
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config: %s", err))
		return
	}

	r.client = opensearch.NewFromConfig(awsCfg)
}

// AuditConfig holds the audit configuration parameters
type AuditConfig struct {
	DisabledRestCategories      []string
	DisabledTransportCategories []string
}

// enableSecurityPluginAuditing enables audit logging via OpenSearch Security API
func enableSecurityPluginAuditing(ctx context.Context, endpoint, username, password string, config AuditConfig) error {
	// Construct the security API URL
	url := fmt.Sprintf("https://%s/_plugins/_security/api/audit/config", endpoint)

	// Audit configuration with hardcoded best-practice settings
	// Only disabled categories are configurable
	auditConfig := map[string]interface{}{
		"enabled": true,
		"audit": map[string]interface{}{
			"enable_rest":                   true,
			"disabled_rest_categories":      config.DisabledRestCategories,
			"enable_transport":              true,
			"disabled_transport_categories": config.DisabledTransportCategories,
			"resolve_bulk_requests":         true,
			"log_request_body":              true,
			"resolve_indices":               true,
			"exclude_sensitive_headers":     true,
			"ignore_users":                  []string{"kibanaserver"},
			"ignore_requests":               []string{},
		},
		"compliance": map[string]interface{}{
			"enabled":               true,
			"internal_config":       true,
			"external_config":       false,
			"read_metadata_only":    true,
			"read_watched_fields":   map[string]interface{}{},
			"read_ignore_users":     []string{"kibanaserver"},
			"write_metadata_only":   true,
			"write_log_diffs":       false,
			"write_watched_indices": []string{},
			"write_ignore_users":    []string{"kibanaserver"},
		},
	}

	jsonData, err := json.Marshal(auditConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal audit config: %w", err)
	}

	// Create HTTP client with TLS config
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}

	// Create PUT request
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	tflog.Info(ctx, "Successfully enabled OpenSearch security plugin auditing", map[string]interface{}{
		"endpoint": endpoint,
		"response": string(body),
	})

	return nil
}

func (r *OpenSearchModifyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OpenSearchModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *opensearch.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = opensearch.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare log publishing options
	logPublishingOptions := make(map[string]types.LogPublishingOption)

	// Add audit logs configuration
	if data.AuditLogsEnabled.ValueBool() {
		logPublishingOptions["AUDIT_LOGS"] = types.LogPublishingOption{
			CloudWatchLogsLogGroupArn: aws.String(data.AuditLogsGroupArn.ValueString()),
			Enabled:                   aws.Bool(true),
		}
	}

	// Add profiler logs configuration if enabled
	if !data.ProfilerLogsEnabled.IsNull() && data.ProfilerLogsEnabled.ValueBool() {
		if !data.ProfilerLogsGroupArn.IsNull() {
			logPublishingOptions["INDEX_SLOW_LOGS"] = types.LogPublishingOption{
				CloudWatchLogsLogGroupArn: aws.String(data.ProfilerLogsGroupArn.ValueString()),
				Enabled:                   aws.Bool(true),
			}
		}
	}

	// Prepare update input
	input := &opensearch.UpdateDomainConfigInput{
		DomainName:           aws.String(data.DomainName.ValueString()),
		LogPublishingOptions: logPublishingOptions,
	}

	tflog.Debug(ctx, "Updating OpenSearch domain configuration", map[string]interface{}{
		"domain_name":         data.DomainName.ValueString(),
		"audit_logs_enabled":  data.AuditLogsEnabled.ValueBool(),
		"profiler_logs_enabled": data.ProfilerLogsEnabled.ValueBool(),
	})

	// Update the OpenSearch domain
	_, err := client.UpdateDomainConfig(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating OpenSearch domain", fmt.Sprintf("Could not update OpenSearch domain: %s", err))
		return
	}

	// Wait for the domain to finish processing using polling
	tflog.Info(ctx, "Waiting for OpenSearch domain to finish processing")
	maxAttempts := 60 // 30 minutes with 30 second intervals
	var domainEndpoint string
	for i := 0; i < maxAttempts; i++ {
		describeInput := &opensearch.DescribeDomainInput{
			DomainName: aws.String(data.DomainName.ValueString()),
		}

		result, err := client.DescribeDomain(ctx, describeInput)
		if err != nil {
			resp.Diagnostics.AddError("Error describing OpenSearch domain", fmt.Sprintf("Could not describe OpenSearch domain: %s", err))
			return
		}

		if result.DomainStatus != nil && result.DomainStatus.Processing != nil && !*result.DomainStatus.Processing {
			tflog.Info(ctx, "OpenSearch domain is no longer processing")
			// Capture the endpoint for security plugin configuration
			if result.DomainStatus.Endpoint != nil {
				domainEndpoint = *result.DomainStatus.Endpoint
			}
			break
		}

		if i == maxAttempts-1 {
			resp.Diagnostics.AddError("Timeout waiting for OpenSearch domain", "OpenSearch domain did not finish processing within 30 minutes")
			return
		}

		time.Sleep(30 * time.Second)
	}

	// Enable security plugin auditing if requested and credentials provided
	if !data.EnableSecurityPluginAuditing.IsNull() && data.EnableSecurityPluginAuditing.ValueBool() {
		if !data.MasterUsername.IsNull() && !data.MasterPassword.IsNull() && domainEndpoint != "" {
			tflog.Info(ctx, "Enabling OpenSearch security plugin auditing")
			
			// Extract disabled REST categories from the list
			var disabledRestCategories []string
			if !data.AuditRestDisabledCategories.IsNull() {
				diags := data.AuditRestDisabledCategories.ElementsAs(ctx, &disabledRestCategories, false)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
			}
			
			// Extract disabled Transport categories from the list
			var disabledTransportCategories []string
			if !data.AuditDisabledTransportCategories.IsNull() {
				diags := data.AuditDisabledTransportCategories.ElementsAs(ctx, &disabledTransportCategories, false)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
			}
			
			// Build audit configuration (only disabled categories are configurable)
			auditConfig := AuditConfig{
				DisabledRestCategories:      disabledRestCategories,
				DisabledTransportCategories: disabledTransportCategories,
			}
			
			tflog.Debug(ctx, "Audit configuration", map[string]interface{}{
				"disabled_rest_categories":      auditConfig.DisabledRestCategories,
				"disabled_transport_categories": auditConfig.DisabledTransportCategories,
			})
			
			err := enableSecurityPluginAuditing(
				ctx,
				domainEndpoint,
				data.MasterUsername.ValueString(),
				data.MasterPassword.ValueString(),
				auditConfig,
			)
			if err != nil {
				resp.Diagnostics.AddWarning(
					"Failed to enable security plugin auditing",
					fmt.Sprintf("CloudWatch logging is enabled, but failed to enable security plugin auditing: %s. You may need to enable it manually via the OpenSearch Dashboard.", err),
				)
			}
		} else {
			resp.Diagnostics.AddWarning(
				"Security plugin auditing not enabled",
				"enable_security_plugin_auditing is true but master_username, master_password, or domain endpoint is missing",
			)
		}
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)
	data.ID = frameworktypes.StringValue(data.DomainName.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OpenSearchModifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OpenSearchModifyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *opensearch.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = opensearch.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Check if the OpenSearch domain exists
	input := &opensearch.DescribeDomainInput{
		DomainName: aws.String(data.DomainName.ValueString()),
	}

	_, err := client.DescribeDomain(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error reading OpenSearch domain", fmt.Sprintf("Could not read OpenSearch domain: %s", err))
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OpenSearchModifyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OpenSearchModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *opensearch.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = opensearch.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare log publishing options
	logPublishingOptions := make(map[string]types.LogPublishingOption)

	// Add audit logs configuration
	if data.AuditLogsEnabled.ValueBool() {
		logPublishingOptions["AUDIT_LOGS"] = types.LogPublishingOption{
			CloudWatchLogsLogGroupArn: aws.String(data.AuditLogsGroupArn.ValueString()),
			Enabled:                   aws.Bool(true),
		}
	}

	// Add profiler logs configuration if enabled
	if !data.ProfilerLogsEnabled.IsNull() && data.ProfilerLogsEnabled.ValueBool() {
		if !data.ProfilerLogsGroupArn.IsNull() {
			logPublishingOptions["INDEX_SLOW_LOGS"] = types.LogPublishingOption{
				CloudWatchLogsLogGroupArn: aws.String(data.ProfilerLogsGroupArn.ValueString()),
				Enabled:                   aws.Bool(true),
			}
		}
	}

	// Prepare update input
	input := &opensearch.UpdateDomainConfigInput{
		DomainName:           aws.String(data.DomainName.ValueString()),
		LogPublishingOptions: logPublishingOptions,
	}

	tflog.Debug(ctx, "Updating OpenSearch domain configuration", map[string]interface{}{
		"domain_name":           data.DomainName.ValueString(),
		"audit_logs_enabled":    data.AuditLogsEnabled.ValueBool(),
		"profiler_logs_enabled": data.ProfilerLogsEnabled.ValueBool(),
	})

	// Update the OpenSearch domain
	_, err := client.UpdateDomainConfig(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating OpenSearch domain", fmt.Sprintf("Could not update OpenSearch domain: %s", err))
		return
	}

	// Wait for the domain to finish processing using polling
	tflog.Debug(ctx, "Waiting for OpenSearch domain to finish processing")
	maxAttempts := 60 // 30 minutes with 30 second intervals
	for i := 0; i < maxAttempts; i++ {
		describeInput := &opensearch.DescribeDomainInput{
			DomainName: aws.String(data.DomainName.ValueString()),
		}

		result, err := client.DescribeDomain(ctx, describeInput)
		if err != nil {
			resp.Diagnostics.AddError("Error describing OpenSearch domain", fmt.Sprintf("Could not describe OpenSearch domain: %s", err))
			return
		}

		if result.DomainStatus != nil && result.DomainStatus.Processing != nil && !*result.DomainStatus.Processing {
			tflog.Debug(ctx, "OpenSearch domain is no longer processing")
			break
		}

		if i == maxAttempts-1 {
			resp.Diagnostics.AddError("Timeout waiting for OpenSearch domain", "OpenSearch domain did not finish processing within 30 minutes")
			return
		}

		time.Sleep(30 * time.Second)
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OpenSearchModifyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *OpenSearchModifyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain_name"), req, resp)
}
