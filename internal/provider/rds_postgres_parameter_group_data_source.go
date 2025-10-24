// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &RDSPostgresParameterGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &RDSPostgresParameterGroupDataSource{}

func NewRDSPostgresParameterGroupDataSource() datasource.DataSource {
	return &RDSPostgresParameterGroupDataSource{}
}

// RDSPostgresParameterGroupDataSource defines the data source implementation.
type RDSPostgresParameterGroupDataSource struct {
	client *rds.Client
}

// RDSPostgresParameterGroupDataSourceModel describes the data source data model.
type RDSPostgresParameterGroupDataSourceModel struct {
	DBIdentifier   types.String `tfsdk:"db_identifier"`
	Region         types.String `tfsdk:"region"`
	ParameterGroup types.String `tfsdk:"parameter_group"`
	FamilyName     types.String `tfsdk:"family_name"`
	Description    types.String `tfsdk:"description"`
	ID             types.String `tfsdk:"id"`
}

func (d *RDSPostgresParameterGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rds_postgres_parameter_group"
}

func (d *RDSPostgresParameterGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Data source for AWS RDS PostgreSQL parameter group",

		Attributes: map[string]schema.Attribute{
			"db_identifier": schema.StringAttribute{
				MarkdownDescription: "RDS PostgreSQL DB instance identifier",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region",
				Optional:            true,
			},
			"parameter_group": schema.StringAttribute{
				MarkdownDescription: "RDS PostgreSQL parameter group name",
				Computed:            true,
			},
			"family_name": schema.StringAttribute{
				MarkdownDescription: "RDS PostgreSQL family name",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "RDS PostgreSQL parameter group description",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source",
			},
		},
	}
}

func (d *RDSPostgresParameterGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring RDS PostgreSQL parameter group data source")

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

	d.client = rds.NewFromConfig(awsCfg)
}

func (d *RDSPostgresParameterGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RDSPostgresParameterGroupDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
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
		client = d.client
	}

	// Get RDS DB instance information
	input := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(data.DBIdentifier.ValueString())}

	tflog.Debug(ctx, "Getting RDS PostgreSQL DB instance information", map[string]interface{}{"db_identifier": data.DBIdentifier.ValueString()})

	result, err := client.DescribeDBInstances(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to describe RDS DB instances", fmt.Sprintf("Error describing RDS DB instances: %s", err))
		return
	}

	if len(result.DBInstances) == 0 {
		resp.Diagnostics.AddError("RDS DB instance not found", fmt.Sprintf("No RDS DB instance found with identifier: %s", data.DBIdentifier.ValueString()))
		return
	}

	instance := result.DBInstances[0]

	// Check if this is a PostgreSQL instance
	if *instance.Engine != "postgres" {
		resp.Diagnostics.AddError("Not a PostgreSQL instance", fmt.Sprintf("The DB instance %s is not a PostgreSQL instance. Engine: %s", data.DBIdentifier.ValueString(), *instance.Engine))
		return
	}

	// Set parameter group value
	data.ParameterGroup = types.StringValue(*instance.DBParameterGroups[0].DBParameterGroupName)
	pgInput := &rds.DescribeDBParameterGroupsInput{
		DBParameterGroupName: instance.DBParameterGroups[0].DBParameterGroupName,
	}

	pgResp, err := client.DescribeDBParameterGroups(ctx, pgInput)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("failed to describe DB parameter groups, %v", err), fmt.Sprintf("failed to describe DB parameter groups, %v", err))
		return
	}

	if len(pgResp.DBParameterGroups) == 0 {
		resp.Diagnostics.AddError(fmt.Sprintf("no parameter group configured for RDS PostgreSQL instance %s", data.DBIdentifier.ValueString()), fmt.Sprintf("no parameter group configured for RDS PostgreSQL instance %s", data.DBIdentifier.ValueString()))
		return
	}

	data.FamilyName = types.StringValue(*pgResp.DBParameterGroups[0].DBParameterGroupFamily)
	data.Description = types.StringValue(*pgResp.DBParameterGroups[0].Description)
	data.ID = types.StringValue(data.DBIdentifier.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
