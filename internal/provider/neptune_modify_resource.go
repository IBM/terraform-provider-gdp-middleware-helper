// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/aws/aws-sdk-go-v2/service/neptune/types"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	frameworktypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &NeptuneModifyResource{}
var _ resource.ResourceWithImportState = &NeptuneModifyResource{}

func NewNeptuneModifyResource() resource.Resource {
	return &NeptuneModifyResource{}
}

// NeptuneModifyResource defines the resource implementation.
type NeptuneModifyResource struct {
	client *neptune.Client
}

// NeptuneModifyResourceModel describes the resource data model.
type NeptuneModifyResourceModel struct {
	ClusterIdentifier         frameworktypes.String `tfsdk:"cluster_identifier"`
	Region                    frameworktypes.String `tfsdk:"region"`
	ClusterParameterGroupName frameworktypes.String `tfsdk:"cluster_parameter_group_name"`
	CloudWatchLogsExports     frameworktypes.List   `tfsdk:"cloudwatch_logs_exports"`
	ApplyImmediately          frameworktypes.Bool   `tfsdk:"apply_immediately"`
	LastModifiedTime          frameworktypes.String `tfsdk:"last_modified_time"`
	ID                        frameworktypes.String `tfsdk:"id"`
}

func (r *NeptuneModifyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_neptune_modify"
}

func (r *NeptuneModifyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for modifying an AWS Neptune cluster configuration",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the Neptune cluster to modify",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the Neptune cluster is located",
				Optional:            true,
			},
			"cluster_parameter_group_name": schema.StringAttribute{
				MarkdownDescription: "The name of the DB cluster parameter group to apply",
				Optional:            true,
			},
			"cloudwatch_logs_exports": schema.ListAttribute{
				MarkdownDescription: "List of log types to enable for exporting to CloudWatch Logs (e.g., 'audit')",
				ElementType:         frameworktypes.StringType,
				Optional:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"apply_immediately": schema.BoolAttribute{
				MarkdownDescription: "Whether to apply changes immediately or during the next maintenance window",
				Optional:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
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

func (r *NeptuneModifyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Neptune modify resource")

	// If provider is not configured, return
	if req.ProviderData == nil {
		return
	}

	// Create AWS config and Neptune client
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config: %s", err))
		return
	}

	r.client = neptune.NewFromConfig(awsCfg)
}

func (r *NeptuneModifyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NeptuneModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *neptune.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = neptune.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare modify input
	input := &neptune.ModifyDBClusterInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	// Set cluster parameter group name if specified
	if !data.ClusterParameterGroupName.IsNull() {
		input.DBClusterParameterGroupName = aws.String(data.ClusterParameterGroupName.ValueString())
	}

	// Set CloudWatch logs exports if specified
	if !data.CloudWatchLogsExports.IsNull() {
		var logsExports []string
		resp.Diagnostics.Append(data.CloudWatchLogsExports.ElementsAs(ctx, &logsExports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		input.CloudwatchLogsExportConfiguration = &types.CloudwatchLogsExportConfiguration{
			EnableLogTypes: logsExports,
		}
	}

	// Set apply immediately if specified
	if !data.ApplyImmediately.IsNull() {
		input.ApplyImmediately = aws.Bool(data.ApplyImmediately.ValueBool())
	}

	tflog.Debug(ctx, "Modifying Neptune cluster", map[string]interface{}{
		"cluster_identifier":           data.ClusterIdentifier.ValueString(),
		"cluster_parameter_group_name": data.ClusterParameterGroupName.ValueString(),
		"apply_immediately":            data.ApplyImmediately.ValueBool(),
	})

	// Modify the Neptune cluster
	_, err := client.ModifyDBCluster(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying Neptune cluster", fmt.Sprintf("Could not modify Neptune cluster: %s", err))
		return
	}

	// Wait for the cluster to become available again using polling
	tflog.Info(ctx, "Waiting for Neptune cluster to become available after modification")
	maxAttempts := 60 // 30 minutes with 30 second intervals
	for i := 0; i < maxAttempts; i++ {
		describeInput := &neptune.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
		}

		result, err := client.DescribeDBClusters(ctx, describeInput)
		if err != nil {
			resp.Diagnostics.AddError("Error describing Neptune cluster", fmt.Sprintf("Could not describe Neptune cluster: %s", err))
			return
		}

		if len(result.DBClusters) > 0 && *result.DBClusters[0].Status == "available" {
			tflog.Info(ctx, "Neptune cluster is now available")
			break
		}

		if i == maxAttempts-1 {
			resp.Diagnostics.AddError("Timeout waiting for Neptune cluster", "Neptune cluster did not become available within 30 minutes")
			return
		}

		time.Sleep(30 * time.Second)
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)
	data.ID = frameworktypes.StringValue(data.ClusterIdentifier.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NeptuneModifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NeptuneModifyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *neptune.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = neptune.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Check if the Neptune cluster exists
	input := &neptune.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	_, err := client.DescribeDBClusters(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Neptune cluster", fmt.Sprintf("Could not read Neptune cluster: %s", err))
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NeptuneModifyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NeptuneModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *neptune.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = neptune.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare modify input
	input := &neptune.ModifyDBClusterInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	// Set cluster parameter group name if specified
	if !data.ClusterParameterGroupName.IsNull() {
		input.DBClusterParameterGroupName = aws.String(data.ClusterParameterGroupName.ValueString())
	}

	// Set CloudWatch logs exports if specified
	if !data.CloudWatchLogsExports.IsNull() {
		var logsExports []string
		resp.Diagnostics.Append(data.CloudWatchLogsExports.ElementsAs(ctx, &logsExports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		input.CloudwatchLogsExportConfiguration = &types.CloudwatchLogsExportConfiguration{
			EnableLogTypes: logsExports,
		}
	}

	// Set apply immediately if specified
	if !data.ApplyImmediately.IsNull() {
		input.ApplyImmediately = aws.Bool(data.ApplyImmediately.ValueBool())
	}

	tflog.Debug(ctx, "Modifying Neptune cluster", map[string]interface{}{
		"cluster_identifier":           data.ClusterIdentifier.ValueString(),
		"cluster_parameter_group_name": data.ClusterParameterGroupName.ValueString(),
		"apply_immediately":            data.ApplyImmediately.ValueBool(),
	})

	// Modify the Neptune cluster
	_, err := client.ModifyDBCluster(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying Neptune cluster", fmt.Sprintf("Could not modify Neptune cluster: %s", err))
		return
	}

	// Wait for the cluster to become available again using polling
	tflog.Debug(ctx, "Waiting for Neptune cluster to become available after modification")
	maxAttempts := 60 // 30 minutes with 30 second intervals
	for i := 0; i < maxAttempts; i++ {
		describeInput := &neptune.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
		}

		result, err := client.DescribeDBClusters(ctx, describeInput)
		if err != nil {
			resp.Diagnostics.AddError("Error describing Neptune cluster", fmt.Sprintf("Could not describe Neptune cluster: %s", err))
			return
		}

		if len(result.DBClusters) > 0 && *result.DBClusters[0].Status == "available" {
			tflog.Debug(ctx, "Neptune cluster is now available")
			break
		}

		if i == maxAttempts-1 {
			resp.Diagnostics.AddError("Timeout waiting for Neptune cluster", "Neptune cluster did not become available within 30 minutes")
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

func (r *NeptuneModifyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *NeptuneModifyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_identifier"), req, resp)
}
