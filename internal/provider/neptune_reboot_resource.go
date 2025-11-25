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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &NeptuneRebootResource{}
var _ resource.ResourceWithImportState = &NeptuneRebootResource{}

func NewNeptuneRebootResource() resource.Resource {
	return &NeptuneRebootResource{}
}

// NeptuneRebootResource defines the resource implementation.
type NeptuneRebootResource struct {
	client *neptune.Client
}

// NeptuneRebootResourceModel describes the resource data model.
type NeptuneRebootResourceModel struct {
	ClusterIdentifier types.String `tfsdk:"cluster_identifier"`
	Region            types.String `tfsdk:"region"`
	LastRebootTime    types.String `tfsdk:"last_reboot_time"`
	ID                types.String `tfsdk:"id"`
}

func (r *NeptuneRebootResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_neptune_reboot"
}

func (r *NeptuneRebootResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for rebooting all instances in an AWS Neptune cluster",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the Neptune cluster whose instances will be rebooted",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the Neptune cluster is located",
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

func (r *NeptuneRebootResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Neptune reboot resource")

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

func (r *NeptuneRebootResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NeptuneRebootResourceModel

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

	// Get all instances in the cluster
	describeInput := &neptune.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	clusterOutput, err := client.DescribeDBClusters(ctx, describeInput)
	if err != nil {
		resp.Diagnostics.AddError("Error describing Neptune cluster", fmt.Sprintf("Could not describe Neptune cluster: %s", err))
		return
	}

	if len(clusterOutput.DBClusters) == 0 {
		resp.Diagnostics.AddError("Neptune cluster not found", fmt.Sprintf("Neptune cluster %s not found", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := clusterOutput.DBClusters[0]
	instances := cluster.DBClusterMembers

	if len(instances) == 0 {
		resp.Diagnostics.AddError("No instances in cluster", fmt.Sprintf("Neptune cluster %s has no instances to reboot", data.ClusterIdentifier.ValueString()))
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Found %d instances in Neptune cluster %s", len(instances), data.ClusterIdentifier.ValueString()))

	// Reboot each instance in the cluster
	for _, member := range instances {
		instanceId := aws.ToString(member.DBInstanceIdentifier)

		tflog.Debug(ctx, "Rebooting Neptune instance", map[string]interface{}{
			"cluster_identifier":  data.ClusterIdentifier.ValueString(),
			"instance_identifier": instanceId,
		})

		rebootInput := &neptune.RebootDBInstanceInput{
			DBInstanceIdentifier: aws.String(instanceId),
		}

		_, err := client.RebootDBInstance(ctx, rebootInput)
		if err != nil {
			resp.Diagnostics.AddError("Error rebooting Neptune instance", fmt.Sprintf("Could not reboot Neptune instance %s: %s", instanceId, err))
			return
		}

		tflog.Info(ctx, fmt.Sprintf("Successfully initiated reboot for Neptune instance: %s", instanceId))
	}

	// Wait for all instances to become available again
	tflog.Info(ctx, "Waiting for all Neptune instances to become available")

	for _, member := range instances {
		instanceId := aws.ToString(member.DBInstanceIdentifier)

		waiter := neptune.NewDBInstanceAvailableWaiter(client)
		waitInput := &neptune.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(instanceId),
		}

		tflog.Debug(ctx, fmt.Sprintf("Waiting for Neptune instance %s to become available", instanceId))
		err = waiter.Wait(ctx, waitInput, 30*time.Minute)
		if err != nil {
			resp.Diagnostics.AddError("Error waiting for Neptune instance to become available", fmt.Sprintf("Could not confirm Neptune instance %s availability: %s", instanceId, err))
			return
		}

		tflog.Info(ctx, fmt.Sprintf("Neptune instance %s is now available", instanceId))
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)
	data.ID = types.StringValue(data.ClusterIdentifier.ValueString())

	tflog.Info(ctx, fmt.Sprintf("Successfully rebooted all instances in Neptune cluster %s", data.ClusterIdentifier.ValueString()))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NeptuneRebootResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NeptuneRebootResourceModel

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

func (r *NeptuneRebootResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NeptuneRebootResourceModel

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

	// Get all instances in the cluster
	describeInput := &neptune.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	clusterOutput, err := client.DescribeDBClusters(ctx, describeInput)
	if err != nil {
		resp.Diagnostics.AddError("Error describing Neptune cluster", fmt.Sprintf("Could not describe Neptune cluster: %s", err))
		return
	}

	if len(clusterOutput.DBClusters) == 0 {
		resp.Diagnostics.AddError("Neptune cluster not found", fmt.Sprintf("Neptune cluster %s not found", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := clusterOutput.DBClusters[0]
	instances := cluster.DBClusterMembers

	// Reboot each instance in the cluster
	for _, member := range instances {
		instanceId := aws.ToString(member.DBInstanceIdentifier)

		tflog.Debug(ctx, "Rebooting Neptune instance", map[string]interface{}{
			"cluster_identifier":  data.ClusterIdentifier.ValueString(),
			"instance_identifier": instanceId,
		})

		rebootInput := &neptune.RebootDBInstanceInput{
			DBInstanceIdentifier: aws.String(instanceId),
		}

		_, err := client.RebootDBInstance(ctx, rebootInput)
		if err != nil {
			resp.Diagnostics.AddError("Error rebooting Neptune instance", fmt.Sprintf("Could not reboot Neptune instance %s: %s", instanceId, err))
			return
		}
	}

	// Wait for all instances to become available again
	tflog.Debug(ctx, "Waiting for all Neptune instances to become available")

	for _, member := range instances {
		instanceId := aws.ToString(member.DBInstanceIdentifier)

		waiter := neptune.NewDBInstanceAvailableWaiter(client)
		waitInput := &neptune.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(instanceId),
		}

		err = waiter.Wait(ctx, waitInput, 30*time.Minute)
		if err != nil {
			resp.Diagnostics.AddError("Error waiting for Neptune instance to become available", fmt.Sprintf("Could not confirm Neptune instance %s availability: %s", instanceId, err))
			return
		}
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NeptuneRebootResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *NeptuneRebootResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_identifier"), req, resp)
}
