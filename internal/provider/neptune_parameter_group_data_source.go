// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/neptune"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &NeptuneParameterGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &NeptuneParameterGroupDataSource{}

func NewNeptuneParameterGroupDataSource() datasource.DataSource {
	return &NeptuneParameterGroupDataSource{}
}

// NeptuneParameterGroupDataSource defines the data source implementation.
type NeptuneParameterGroupDataSource struct {
	client *neptune.Client
}

// NeptuneParameterGroupDataSourceModel describes the data source data model.
type NeptuneParameterGroupDataSourceModel struct {
	ClusterIdentifier types.String `tfsdk:"cluster_identifier"`
	Region            types.String `tfsdk:"region"`
	ParameterGroup    types.String `tfsdk:"parameter_group"`
	FamilyName        types.String `tfsdk:"family_name"`
	Description       types.String `tfsdk:"description"`
	ID                types.String `tfsdk:"id"`
}

func (d *NeptuneParameterGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_neptune_parameter_group"
}

func (d *NeptuneParameterGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Data source for AWS Neptune cluster parameter group",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "Neptune DB cluster identifier",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region",
				Optional:            true,
			},
			"parameter_group": schema.StringAttribute{
				MarkdownDescription: "Neptune cluster parameter group name",
				Computed:            true,
			},
			"family_name": schema.StringAttribute{
				MarkdownDescription: "Neptune parameter group family name",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Neptune cluster parameter group description",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source",
			},
		},
	}
}

func (d *NeptuneParameterGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Neptune parameter group data source")

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

	d.client = neptune.NewFromConfig(awsCfg)
}

func (d *NeptuneParameterGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NeptuneParameterGroupDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
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
		client = d.client
	}

	// Get Neptune DB cluster information
	input := &neptune.DescribeDBClustersInput{DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString())}

	tflog.Debug(ctx, "Getting Neptune DB cluster information", map[string]interface{}{"cluster_identifier": data.ClusterIdentifier.ValueString()})

	result, err := client.DescribeDBClusters(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to describe Neptune DB clusters", fmt.Sprintf("Error describing Neptune DB clusters: %s", err))
		return
	}

	if len(result.DBClusters) == 0 {
		resp.Diagnostics.AddError("Neptune DB cluster not found", fmt.Sprintf("No Neptune DB cluster found with identifier: %s", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := result.DBClusters[0]

	// Check if this is a Neptune cluster
	if *cluster.Engine != "neptune" {
		resp.Diagnostics.AddError("Not a Neptune cluster", fmt.Sprintf("The DB cluster %s is not a Neptune cluster. Engine: %s", data.ClusterIdentifier.ValueString(), *cluster.Engine))
		return
	}

	// Set parameter group value
	data.ParameterGroup = types.StringValue(*cluster.DBClusterParameterGroup)
	pgInput := &neptune.DescribeDBClusterParameterGroupsInput{
		DBClusterParameterGroupName: cluster.DBClusterParameterGroup,
	}

	pgResp, err := client.DescribeDBClusterParameterGroups(ctx, pgInput)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("failed to describe DB cluster parameter groups, %v", err), fmt.Sprintf("failed to describe DB cluster parameter groups, %v", err))
		return
	}

	if len(pgResp.DBClusterParameterGroups) == 0 {
		resp.Diagnostics.AddError(fmt.Sprintf("no parameter group configured for Neptune cluster %s", data.ClusterIdentifier.ValueString()), fmt.Sprintf("no parameter group configured for Neptune cluster %s", data.ClusterIdentifier.ValueString()))
		return
	}

	data.FamilyName = types.StringValue(*pgResp.DBClusterParameterGroups[0].DBParameterGroupFamily)
	data.Description = types.StringValue(*pgResp.DBClusterParameterGroups[0].Description)
	data.ID = types.StringValue(data.ClusterIdentifier.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
