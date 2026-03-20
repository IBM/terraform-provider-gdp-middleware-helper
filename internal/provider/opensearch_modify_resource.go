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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
		MarkdownDescription: "Resource for enabling OpenSearch security plugin auditing",

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

// normalizeCategories converts category names with spaces to underscores
// OpenSearch expects categories like "FAILED_LOGIN" not "FAILED LOGIN"
func normalizeCategories(categories []string) []string {
	normalized := make([]string, len(categories))
	for i, cat := range categories {
		// Replace spaces with underscores and ensure uppercase
		normalized[i] = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(cat)), " ", "_")
	}
	return normalized
}

// enableSecurityPluginAuditing enables audit logging via OpenSearch Security API
func enableSecurityPluginAuditing(ctx context.Context, endpoint, username, password string, config AuditConfig) error {
	// Construct the security API URL
	url := fmt.Sprintf("https://%s/_plugins/_security/api/audit/config", endpoint)

	// Normalize category names (replace spaces with underscores)
	normalizedRestCategories := normalizeCategories(config.DisabledRestCategories)
	normalizedTransportCategories := normalizeCategories(config.DisabledTransportCategories)

	tflog.Debug(ctx, "Normalized audit categories", map[string]interface{}{
		"original_rest_categories":       config.DisabledRestCategories,
		"normalized_rest_categories":     normalizedRestCategories,
		"original_transport_categories":  config.DisabledTransportCategories,
		"normalized_transport_categories": normalizedTransportCategories,
	})

	// Audit configuration with hardcoded best-practice settings
	// Only disabled categories are configurable
	auditConfig := map[string]interface{}{
		"enabled": true,
		"audit": map[string]interface{}{
			"enable_rest":                   true,
			"disabled_rest_categories":      normalizedRestCategories,
			"enable_transport":              true,
			"disabled_transport_categories": normalizedTransportCategories,
			"resolve_bulk_requests":         true,
			"log_request_body":              true,
			"resolve_indices":               true,
			"exclude_sensitive_headers":     true,
			"ignore_users":                  []string{},
			"ignore_requests":               []string{},
		},
		"compliance": map[string]interface{}{
			"enabled":               true,
			"internal_config":       true,
			"external_config":       false,
			"read_metadata_only":    true,
			"read_watched_fields":   map[string]interface{}{},
			"read_ignore_users":     []string{},
			"write_metadata_only":   true,
			"write_log_diffs":       false,
			"write_watched_indices": []string{},
			"write_ignore_users":    []string{},
		},
	}

	jsonData, err := json.Marshal(auditConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal audit config: %w", err)
	}

	tflog.Debug(ctx, "Audit config JSON payload", map[string]interface{}{
		"payload": string(jsonData),
	})

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

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

	// Process the domain modification
	r.processDomainModification(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set computed values
	data.LastModifiedTime = frameworktypes.StringValue(time.Now().Format(time.RFC3339))
	data.ID = frameworktypes.StringValue(data.DomainName.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// getClient returns an OpenSearch client, optionally configured with a specific region
func (r *OpenSearchModifyResource) getClient(ctx context.Context, region frameworktypes.String, diags *diag.Diagnostics) *opensearch.Client {
	if !region.IsNull() {
		tflog.Debug(ctx, "Configuring client with region", map[string]interface{}{"region": region.ValueString()})
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region.ValueString()))
		if err != nil {
			diags.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", region.ValueString(), err))
			return nil
		}
		return opensearch.NewFromConfig(awsCfg)
	}
	tflog.Debug(ctx, "Using default client")
	return r.client
}

// waitForDomainReady waits for the OpenSearch domain to finish processing and returns its endpoint
func (r *OpenSearchModifyResource) waitForDomainReady(ctx context.Context, client *opensearch.Client, domainName string, diags *diag.Diagnostics) string {
	tflog.Info(ctx, "Waiting for OpenSearch domain to finish processing")
	maxAttempts := 120 // 30 minutes with 15 second intervals

	for i := 0; i < maxAttempts; i++ {
		result, err := client.DescribeDomain(ctx, &opensearch.DescribeDomainInput{
			DomainName: aws.String(domainName),
		})
		if err != nil {
			diags.AddError("Error describing OpenSearch domain", fmt.Sprintf("Could not describe OpenSearch domain: %s", err))
			return ""
		}

		if result.DomainStatus != nil && result.DomainStatus.Processing != nil && !*result.DomainStatus.Processing {
			tflog.Info(ctx, "OpenSearch domain is ready")
			if result.DomainStatus.Endpoint != nil {
				return *result.DomainStatus.Endpoint
			}
			diags.AddWarning("OpenSearch domain ready but endpoint not available", "Domain finished processing but endpoint is not set")
			return ""
		}

		if i == maxAttempts-1 {
			diags.AddError("Timeout waiting for OpenSearch domain", "OpenSearch domain did not finish processing within 30 minutes")
			return ""
		}

		time.Sleep(15 * time.Second)
	}

	return ""
}

// enableSecurityAuditing enables OpenSearch security plugin auditing if configured
func (r *OpenSearchModifyResource) enableSecurityAuditing(ctx context.Context, data *OpenSearchModifyResourceModel, domainEndpoint string, diags *diag.Diagnostics) {
	if data.EnableSecurityPluginAuditing.IsNull() || !data.EnableSecurityPluginAuditing.ValueBool() {
		return
	}

	if data.MasterUsername.IsNull() || data.MasterPassword.IsNull() || domainEndpoint == "" {
		diags.AddWarning(
			"Security plugin auditing not enabled",
			"enable_security_plugin_auditing is true but master_username, master_password, or domain endpoint is missing",
		)
		return
	}

	tflog.Info(ctx, "Enabling OpenSearch security plugin auditing")

	// Extract disabled categories
	var disabledRestCategories []string
	if !data.AuditRestDisabledCategories.IsNull() {
		if diagsTemp := data.AuditRestDisabledCategories.ElementsAs(ctx, &disabledRestCategories, false); diagsTemp.HasError() {
			diags.Append(diagsTemp...)
			return
		}
	}

	var disabledTransportCategories []string
	if !data.AuditDisabledTransportCategories.IsNull() {
		if diagsTemp := data.AuditDisabledTransportCategories.ElementsAs(ctx, &disabledTransportCategories, false); diagsTemp.HasError() {
			diags.Append(diagsTemp...)
			return
		}
	}

	auditConfig := AuditConfig{
		DisabledRestCategories:      disabledRestCategories,
		DisabledTransportCategories: disabledTransportCategories,
	}

	tflog.Debug(ctx, "Audit configuration", map[string]interface{}{
		"disabled_rest_categories":      auditConfig.DisabledRestCategories,
		"disabled_transport_categories": auditConfig.DisabledTransportCategories,
	})

	if err := enableSecurityPluginAuditing(ctx, domainEndpoint, data.MasterUsername.ValueString(), data.MasterPassword.ValueString(), auditConfig); err != nil {
		diags.AddWarning(
			"Failed to enable security plugin auditing",
			fmt.Sprintf("Failed to enable security plugin auditing: %s. You may need to enable it manually via the OpenSearch Dashboard.", err),
		)
	}
}

// processDomainModification handles the common logic for Create and Update operations
func (r *OpenSearchModifyResource) processDomainModification(ctx context.Context, data *OpenSearchModifyResourceModel, diags *diag.Diagnostics) {
	// Get AWS client with optional region override
	client := r.getClient(ctx, data.Region, diags)
	if diags.HasError() {
		return
	}

	// Wait for domain to be ready and get endpoint
	domainEndpoint := r.waitForDomainReady(ctx, client, data.DomainName.ValueString(), diags)
	if diags.HasError() {
		return
	}

	// Enable security plugin auditing if requested
	r.enableSecurityAuditing(ctx, data, domainEndpoint, diags)
}

func (r *OpenSearchModifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OpenSearchModifyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get AWS client with optional region override
	client := r.getClient(ctx, data.Region, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
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

	// Process the domain modification
	r.processDomainModification(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set computed values
	data.LastModifiedTime = frameworktypes.StringValue(time.Now().Format(time.RFC3339))

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
