// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
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
var _ resource.Resource = &AuroraModifyResource{}
var _ resource.ResourceWithImportState = &AuroraModifyResource{}

func NewAuroraModifyResource() resource.Resource {
	return &AuroraModifyResource{}
}

// AuroraModifyResource defines the resource implementation.
type AuroraModifyResource struct {
	client *rds.Client
}

// AuroraModifyResourceModel describes the resource data model.
type AuroraModifyResourceModel struct {
	ClusterIdentifier     frameworktypes.String `tfsdk:"cluster_identifier"`
	Region                frameworktypes.String `tfsdk:"region"`
	ParameterGroupName    frameworktypes.String `tfsdk:"parameter_group_name"`
	CloudWatchLogsExports frameworktypes.List   `tfsdk:"cloudwatch_logs_exports"`
	ApplyImmediately      frameworktypes.Bool   `tfsdk:"apply_immediately"`
	LastModifiedTime      frameworktypes.String `tfsdk:"last_modified_time"`
	ID                    frameworktypes.String `tfsdk:"id"`
}

func (r *AuroraModifyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aurora_modify"
}

func (r *AuroraModifyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for modifying an AWS Aurora cluster configuration",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the Aurora cluster to modify",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the Aurora cluster is located",
				Optional:            true,
			},
			"parameter_group_name": schema.StringAttribute{
				MarkdownDescription: "The name of the DB cluster parameter group to apply",
				Optional:            true,
			},
			"cloudwatch_logs_exports": schema.ListAttribute{
				MarkdownDescription: "List of log types to enable for exporting to CloudWatch Logs (e.g., audit, error, general, slowquery)",
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

func (r *AuroraModifyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Aurora modify resource")

	// If provider is not configured, return
	if req.ProviderData == nil {
		return
	}

	// Create AWS config and RDS client
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config: %s", err))
		return
	}

	r.client = rds.NewFromConfig(awsCfg)
}

func (r *AuroraModifyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AuroraModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *rds.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = rds.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare modify input
	input := &rds.ModifyDBClusterInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	// Set parameter group name if specified
	if !data.ParameterGroupName.IsNull() {
		input.DBClusterParameterGroupName = aws.String(data.ParameterGroupName.ValueString())
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

	tflog.Debug(ctx, "Modifying Aurora cluster", map[string]interface{}{
		"cluster_identifier":   data.ClusterIdentifier.ValueString(),
		"parameter_group_name": data.ParameterGroupName.ValueString(),
		"apply_immediately":    data.ApplyImmediately.ValueBool(),
	})

	// Modify the Aurora cluster
	_, err := client.ModifyDBCluster(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying Aurora cluster", fmt.Sprintf("Could not modify Aurora cluster: %s", err))
		return
	}

	// Wait for the cluster to become available again
	waiter := rds.NewDBClusterAvailableWaiter(client)
	waitInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	tflog.Info(ctx, "Waiting for Aurora cluster to become available after modification")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for Aurora cluster to become available", fmt.Sprintf("Could not confirm Aurora cluster availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)
	data.ID = frameworktypes.StringValue(data.ClusterIdentifier.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuroraModifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuroraModifyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *rds.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = rds.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Check if the Aurora cluster exists
	input := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	_, err := client.DescribeDBClusters(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Aurora cluster", fmt.Sprintf("Could not read Aurora cluster: %s", err))
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuroraModifyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuroraModifyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *rds.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = rds.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = r.client
	}

	// Prepare modify input
	input := &rds.ModifyDBClusterInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	// Set parameter group name if specified
	if !data.ParameterGroupName.IsNull() {
		input.DBClusterParameterGroupName = aws.String(data.ParameterGroupName.ValueString())
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

	tflog.Debug(ctx, "Modifying Aurora cluster", map[string]interface{}{
		"cluster_identifier":   data.ClusterIdentifier.ValueString(),
		"parameter_group_name": data.ParameterGroupName.ValueString(),
		"apply_immediately":    data.ApplyImmediately.ValueBool(),
	})

	// Modify the Aurora cluster
	_, err := client.ModifyDBCluster(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying Aurora cluster", fmt.Sprintf("Could not modify Aurora cluster: %s", err))
		return
	}

	// Wait for the cluster to become available again
	waiter := rds.NewDBClusterAvailableWaiter(client)
	waitInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	tflog.Debug(ctx, "Waiting for Aurora cluster to become available after modification")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for Aurora cluster to become available", fmt.Sprintf("Could not confirm Aurora cluster availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuroraModifyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *AuroraModifyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_identifier"), req, resp)
}
