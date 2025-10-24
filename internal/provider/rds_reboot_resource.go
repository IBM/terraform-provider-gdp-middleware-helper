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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &RDSRebootResource{}
var _ resource.ResourceWithImportState = &RDSRebootResource{}

func NewRDSRebootResource() resource.Resource {
	return &RDSRebootResource{}
}

// RDSRebootResource defines the resource implementation.
type RDSRebootResource struct {
	client *rds.Client
}

// RDSRebootResourceModel describes the resource data model.
type RDSRebootResourceModel struct {
	DBInstanceIdentifier types.String `tfsdk:"db_instance_identifier"`
	Region               types.String `tfsdk:"region"`
	ForceFailover        types.Bool   `tfsdk:"force_failover"`
	LastRebootTime       types.String `tfsdk:"last_reboot_time"`
	ID                   types.String `tfsdk:"id"`
}

func (r *RDSRebootResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rds_reboot"
}

func (r *RDSRebootResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for rebooting an AWS RDS instance",

		Attributes: map[string]schema.Attribute{
			"db_instance_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the RDS instance to reboot",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the RDS instance is located",
				Optional:            true,
			},
			"force_failover": schema.BoolAttribute{
				MarkdownDescription: "When true, the reboot is conducted through a MultiAZ failover",
				Optional:            true,
			},
			"last_reboot_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last reboot operation",
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

func (r *RDSRebootResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring RDS reboot resource")

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

func (r *RDSRebootResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RDSRebootResourceModel

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

	// Prepare reboot input
	input := &rds.RebootDBInstanceInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	// Set force failover if specified
	if !data.ForceFailover.IsNull() {
		input.ForceFailover = aws.Bool(data.ForceFailover.ValueBool())
	}

	tflog.Debug(ctx, "Rebooting RDS instance", map[string]interface{}{
		"db_instance_identifier": data.DBInstanceIdentifier.ValueString(),
		"force_failover":         data.ForceFailover.ValueBool(),
	})

	// Reboot the RDS instance
	_, err := client.RebootDBInstance(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error rebooting RDS instance", fmt.Sprintf("Could not reboot RDS instance: %s", err))
		return
	}

	// Wait for the instance to become available again
	waiter := rds.NewDBInstanceAvailableWaiter(client)
	waitInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	tflog.Info(ctx, "Waiting for RDS instance to become available")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for RDS instance to become available", fmt.Sprintf("Could not confirm RDS instance availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)
	data.ID = types.StringValue(data.DBInstanceIdentifier.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RDSRebootResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RDSRebootResourceModel

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

func (r *RDSRebootResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RDSRebootResourceModel

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

	// Prepare reboot input
	input := &rds.RebootDBInstanceInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	// Set force failover if specified
	if !data.ForceFailover.IsNull() {
		input.ForceFailover = aws.Bool(data.ForceFailover.ValueBool())
	}

	tflog.Debug(ctx, "Rebooting RDS instance", map[string]interface{}{
		"db_instance_identifier": data.DBInstanceIdentifier.ValueString(),
		"force_failover":         data.ForceFailover.ValueBool(),
	})

	// Reboot the RDS instance
	_, err := client.RebootDBInstance(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error rebooting RDS instance", fmt.Sprintf("Could not reboot RDS instance: %s", err))
		return
	}

	// Wait for the instance to become available again
	waiter := rds.NewDBInstanceAvailableWaiter(client)
	waitInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(data.DBInstanceIdentifier.ValueString()),
	}

	tflog.Debug(ctx, "Waiting for RDS instance to become available")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for RDS instance to become available", fmt.Sprintf("Could not confirm RDS instance availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RDSRebootResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *RDSRebootResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("db_instance_identifier"), req, resp)
}
