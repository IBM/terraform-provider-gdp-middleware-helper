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
var _ resource.Resource = &AuroraRebootResource{}
var _ resource.ResourceWithImportState = &AuroraRebootResource{}

func NewAuroraRebootResource() resource.Resource {
	return &AuroraRebootResource{}
}

// AuroraRebootResource defines the resource implementation.
type AuroraRebootResource struct {
	client *rds.Client
}

// AuroraRebootResourceModel describes the resource data model.
type AuroraRebootResourceModel struct {
	ClusterIdentifier types.String `tfsdk:"cluster_identifier"`
	Region            types.String `tfsdk:"region"`
	ForceFailover     types.Bool   `tfsdk:"force_failover"`
	LastRebootTime    types.String `tfsdk:"last_reboot_time"`
	ID                types.String `tfsdk:"id"`
}

func (r *AuroraRebootResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aurora_reboot"
}

func (r *AuroraRebootResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource for rebooting an AWS Aurora PostgreSQL cluster",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier of the Aurora PostgreSQL cluster to reboot",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region where the Aurora PostgreSQL cluster is located",
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

func (r *AuroraRebootResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Aurora reboot resource")

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

func (r *AuroraRebootResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AuroraRebootResourceModel

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

	// Get the list of instances in the cluster to determine reboot strategy
	describeInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	describeResult, err := client.DescribeDBClusters(ctx, describeInput)
	if err != nil {
		resp.Diagnostics.AddError("Error describing Aurora cluster", fmt.Sprintf("Could not describe Aurora cluster: %s", err))
		return
	}

	if len(describeResult.DBClusters) == 0 {
		resp.Diagnostics.AddError("Aurora cluster not found", fmt.Sprintf("No Aurora cluster found with identifier: %s", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := describeResult.DBClusters[0]

	// Count total instances in the cluster
	instanceCount := len(cluster.DBClusterMembers)

	// Check if there are multiple instances (writer + reader)
	hasReaderInstance := false
	var readerInstanceID *string
	for _, member := range cluster.DBClusterMembers {
		if !*member.IsClusterWriter {
			hasReaderInstance = true
			readerInstanceID = member.DBInstanceIdentifier
			break
		}
	}

	// Only use failover if:
	// 1. There are at least 2 instances in the cluster
	// 2. There's a reader instance available
	// 3. force_failover is explicitly set to true
	if instanceCount >= 2 && hasReaderInstance && !data.ForceFailover.IsNull() && data.ForceFailover.ValueBool() {
		// Prepare failover input
		input := &rds.FailoverDBClusterInput{
			DBClusterIdentifier:        aws.String(data.ClusterIdentifier.ValueString()),
			TargetDBInstanceIdentifier: readerInstanceID,
		}

		tflog.Debug(ctx, "Failing over Aurora cluster", map[string]interface{}{
			"cluster_identifier": data.ClusterIdentifier.ValueString(),
			"target_instance":    *readerInstanceID,
			"instance_count":     instanceCount,
		})

		// Failover the Aurora cluster
		_, err = client.FailoverDBCluster(ctx, input)
		if err != nil {
			resp.Diagnostics.AddError("Error failing over Aurora cluster", fmt.Sprintf("Could not failover Aurora cluster: %s", err))
			return
		}
	} else {
		// Reboot each instance in the cluster individually
		tflog.Debug(ctx, "Rebooting Aurora cluster instances individually", map[string]interface{}{
			"cluster_identifier": data.ClusterIdentifier.ValueString(),
			"instance_count":     instanceCount,
			"reason":             "Single instance cluster or force_failover not enabled",
		})

		for _, member := range cluster.DBClusterMembers {
			rebootInput := &rds.RebootDBInstanceInput{
				DBInstanceIdentifier: member.DBInstanceIdentifier,
			}

			// Don't set ForceFailover for single-instance clusters
			if instanceCount >= 2 && !data.ForceFailover.IsNull() {
				rebootInput.ForceFailover = aws.Bool(data.ForceFailover.ValueBool())
			}

			tflog.Debug(ctx, "Rebooting instance", map[string]interface{}{
				"instance_identifier": *member.DBInstanceIdentifier,
			})

			_, err = client.RebootDBInstance(ctx, rebootInput)
			if err != nil {
				resp.Diagnostics.AddError("Error rebooting Aurora instance", fmt.Sprintf("Could not reboot Aurora instance %s: %s", *member.DBInstanceIdentifier, err))
				return
			}
		}
	}

	// Wait for the cluster to become available again
	waiter := rds.NewDBClusterAvailableWaiter(client)
	waitInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	tflog.Info(ctx, "Waiting for Aurora cluster to become available")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for Aurora cluster to become available", fmt.Sprintf("Could not confirm Aurora cluster availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)
	data.ID = types.StringValue(data.ClusterIdentifier.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuroraRebootResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuroraRebootResourceModel

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

func (r *AuroraRebootResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AuroraRebootResourceModel

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

	// Get the list of instances in the cluster to determine reboot strategy
	describeInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	describeResult, err := client.DescribeDBClusters(ctx, describeInput)
	if err != nil {
		resp.Diagnostics.AddError("Error describing Aurora cluster", fmt.Sprintf("Could not describe Aurora cluster: %s", err))
		return
	}

	if len(describeResult.DBClusters) == 0 {
		resp.Diagnostics.AddError("Aurora cluster not found", fmt.Sprintf("No Aurora cluster found with identifier: %s", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := describeResult.DBClusters[0]

	// Count total instances in the cluster
	instanceCount := len(cluster.DBClusterMembers)

	// Check if there are multiple instances (writer + reader)
	hasReaderInstance := false
	var readerInstanceID *string
	for _, member := range cluster.DBClusterMembers {
		if !*member.IsClusterWriter {
			hasReaderInstance = true
			readerInstanceID = member.DBInstanceIdentifier
			break
		}
	}

	// Only use failover if:
	// 1. There are at least 2 instances in the cluster
	// 2. There's a reader instance available
	// 3. force_failover is explicitly set to true
	if instanceCount >= 2 && hasReaderInstance && !data.ForceFailover.IsNull() && data.ForceFailover.ValueBool() {
		// Prepare failover input
		input := &rds.FailoverDBClusterInput{
			DBClusterIdentifier:        aws.String(data.ClusterIdentifier.ValueString()),
			TargetDBInstanceIdentifier: readerInstanceID,
		}

		tflog.Debug(ctx, "Failing over Aurora cluster", map[string]interface{}{
			"cluster_identifier": data.ClusterIdentifier.ValueString(),
			"target_instance":    *readerInstanceID,
			"instance_count":     instanceCount,
		})

		// Failover the Aurora cluster
		_, err = client.FailoverDBCluster(ctx, input)
		if err != nil {
			resp.Diagnostics.AddError("Error failing over Aurora cluster", fmt.Sprintf("Could not failover Aurora cluster: %s", err))
			return
		}
	} else {
		// Reboot each instance in the cluster individually
		tflog.Debug(ctx, "Rebooting Aurora cluster instances individually", map[string]interface{}{
			"cluster_identifier": data.ClusterIdentifier.ValueString(),
			"instance_count":     instanceCount,
			"reason":             "Single instance cluster or force_failover not enabled",
		})

		for _, member := range cluster.DBClusterMembers {
			rebootInput := &rds.RebootDBInstanceInput{
				DBInstanceIdentifier: member.DBInstanceIdentifier,
			}

			// Don't set ForceFailover for single-instance clusters
			if instanceCount >= 2 && !data.ForceFailover.IsNull() {
				rebootInput.ForceFailover = aws.Bool(data.ForceFailover.ValueBool())
			}

			tflog.Debug(ctx, "Rebooting instance", map[string]interface{}{
				"instance_identifier": *member.DBInstanceIdentifier,
			})

			_, err = client.RebootDBInstance(ctx, rebootInput)
			if err != nil {
				resp.Diagnostics.AddError("Error rebooting Aurora instance", fmt.Sprintf("Could not reboot Aurora instance %s: %s", *member.DBInstanceIdentifier, err))
				return
			}
		}
	}

	// Wait for the cluster to become available again
	waiter := rds.NewDBClusterAvailableWaiter(client)
	waitInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString()),
	}

	tflog.Debug(ctx, "Waiting for Aurora cluster to become available")
	err = waiter.Wait(ctx, waitInput, 30*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for Aurora cluster to become available", fmt.Sprintf("Could not confirm Aurora cluster availability: %s", err))
		return
	}

	// Set computed values
	currentTime := time.Now().Format(time.RFC3339)
	data.LastRebootTime = types.StringValue(currentTime)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AuroraRebootResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed on delete - this is a stateless operation
	// The resource will be removed from state
}

func (r *AuroraRebootResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("cluster_identifier"), req, resp)
}
