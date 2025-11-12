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
var _ resource.Resource = &RDSModifyResource{}
var _ resource.ResourceWithImportState = &RDSModifyResource{}

func NewRDSModifyResource() resource.Resource {
	return &RDSModifyResource{}
}

// RDSModifyResource defines the resource implementation.
type RDSModifyResource struct {
	client *rds.Client
}

// RDSModifyResourceModel describes the resource data model.
type RDSModifyResourceModel struct {
	DBInstanceIdentifier  frameworktypes.String `tfsdk:"db_instance_identifier"`
	Region                frameworktypes.String `tfsdk:"region"`
	ParameterGroupName    frameworktypes.String `tfsdk:"parameter_group_name"`
	OptionGroupName       frameworktypes.String `tfsdk:"option_group_name"`
	CloudWatchLogsExports frameworktypes.List   `tfsdk:"cloudwatch_logs_exports"`
	ApplyImmediately      frameworktypes.Bool   `tfsdk:"apply_immediately"`
	LastModifiedTime      frameworktypes.String `tfsdk:"last_modified_time"`
	ID                    frameworktypes.String `tfsdk:"id"`
}

func (r *RDSModifyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rds_modify"
}

func (r *RDSModifyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for modifying an AWS RDS instance configuration",

		Attributes: map[string]schema.Attribute{
			"db_instance_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the RDS instance to modify",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the RDS instance is located",
				Optional:            true,
			},
			"parameter_group_name": schema.StringAttribute{
				MarkdownDescription: "The name of the DB parameter group to apply",
				Optional:            true,
			},
			"option_group_name": schema.StringAttribute{
				MarkdownDescription: "The name of the option group to apply",
				Optional:            true,
			},
			"cloudwatch_logs_exports": schema.ListAttribute{
				MarkdownDescription: "List of log types to enable for exporting to CloudWatch Logs",
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

func (r *RDSModifyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring RDS modify resource")

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

func (r *RDSModifyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RDSModifyResourceModel

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
	input := &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	// Set parameter group name if specified
	if !data.ParameterGroupName.IsNull() {
		input.DBParameterGroupName = aws.String(data.ParameterGroupName.ValueString())
	}

	// Set option group name if specified
	if !data.OptionGroupName.IsNull() {
		input.OptionGroupName = aws.String(data.OptionGroupName.ValueString())
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

	tflog.Debug(ctx, "Modifying RDS instance", map[string]interface{}{
		"db_instance_identifier": data.DBInstanceIdentifier.ValueString(),
		"parameter_group_name":   data.ParameterGroupName.ValueString(),
		"option_group_name":      data.OptionGroupName.ValueString(),
		"apply_immediately":      data.ApplyImmediately.ValueBool(),
	})

	// Modify the RDS instance
	_, err := client.ModifyDBInstance(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying RDS instance", fmt.Sprintf("Could not modify RDS instance: %s", err))
		return
	}

	// Wait for the instance to become available again
	waiter := rds.NewDBInstanceAvailableWaiter(client)
	waitInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	tflog.Info(ctx, "Waiting for RDS instance to become available after modification")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for RDS instance to become available", fmt.Sprintf("Could not confirm RDS instance availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)
	data.ID = frameworktypes.StringValue(data.DBInstanceIdentifier.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RDSModifyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RDSModifyResourceModel

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

	// Check if the RDS instance exists
	input := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	_, err := client.DescribeDBInstances(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error reading RDS instance", fmt.Sprintf("Could not read RDS instance: %s", err))
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RDSModifyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RDSModifyResourceModel

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
	input := &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	// Set parameter group name if specified
	if !data.ParameterGroupName.IsNull() {
		input.DBParameterGroupName = aws.String(data.ParameterGroupName.ValueString())
	}

	// Set option group name if specified
	if !data.OptionGroupName.IsNull() {
		input.OptionGroupName = aws.String(data.OptionGroupName.ValueString())
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

	tflog.Debug(ctx, "Modifying RDS instance", map[string]interface{}{
		"db_instance_identifier": data.DBInstanceIdentifier.ValueString(),
		"parameter_group_name":   data.ParameterGroupName.ValueString(),
		"option_group_name":      data.OptionGroupName.ValueString(),
		"apply_immediately":      data.ApplyImmediately.ValueBool(),
	})

	// Modify the RDS instance
	_, err := client.ModifyDBInstance(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error modifying RDS instance", fmt.Sprintf("Could not modify RDS instance: %s", err))
		return
	}

	// Wait for the instance to become available again
	waiter := rds.NewDBInstanceAvailableWaiter(client)
	waitInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	tflog.Debug(ctx, "Waiting for RDS instance to become available after modification")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for RDS instance to become available", fmt.Sprintf("Could not confirm RDS instance availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastModifiedTime = frameworktypes.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RDSModifyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *RDSModifyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("db_instance_identifier"), req, resp)
}
