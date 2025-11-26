// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &RDSMSSQLDataSource{}
var _ datasource.DataSourceWithConfigure = &RDSMSSQLDataSource{}

func NewRDSMSSQLDataSource() datasource.DataSource {
	return &RDSMSSQLDataSource{}
}

// RDSMSSQLDataSource defines the data source implementation.
type RDSMSSQLDataSource struct {
	client *rds.Client
}

// MSSQLOptionModel represents an individual option in the option group
type MSSQLOptionModel struct {
	OptionName        types.String `tfsdk:"option_name"`
	OptionDescription types.String `tfsdk:"option_description"`
	Permanent         types.Bool   `tfsdk:"permanent"`
	Persistent        types.Bool   `tfsdk:"persistent"`
	Port              types.Int64  `tfsdk:"port"`
}

// RDSMSSQLDataSourceModel describes the data source data model.
type RDSMSSQLDataSourceModel struct {
	DBIdentifier types.String `tfsdk:"db_identifier"`
	Region       types.String `tfsdk:"region"`
	ID           types.String `tfsdk:"id"`

	// Parameter Group attributes
	ParameterGroup types.String `tfsdk:"parameter_group"`
	FamilyName     types.String `tfsdk:"family_name"`
	PGDescription  types.String `tfsdk:"parameter_group_description"`

	// Option Group attributes
	OptionGroup   types.String `tfsdk:"option_group"`
	EngineName    types.String `tfsdk:"engine_name"`
	MajorVersion  types.String `tfsdk:"major_version"`
	OGDescription types.String `tfsdk:"option_group_description"`
	Options       types.List   `tfsdk:"options"`
}

func (d *RDSMSSQLDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rds_mssql"
}

func (d *RDSMSSQLDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Data source for AWS RDS SQL Server parameter and option groups",

		Attributes: map[string]schema.Attribute{
			"db_identifier": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server DB instance identifier",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source",
			},

			// Parameter Group attributes
			"parameter_group": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server parameter group name",
				Computed:            true,
			},
			"family_name": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server parameter group family name",
				Computed:            true,
			},
			"parameter_group_description": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server parameter group description",
				Computed:            true,
			},

			// Option Group attributes
			"option_group": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server option group name",
				Computed:            true,
			},
			"engine_name": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server engine name",
				Computed:            true,
			},
			"major_version": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server major version",
				Computed:            true,
			},
			"option_group_description": schema.StringAttribute{
				MarkdownDescription: "RDS SQL Server option group description",
				Computed:            true,
			},
			"options": schema.ListNestedAttribute{
				MarkdownDescription: "List of options in the option group",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"option_name": schema.StringAttribute{
							MarkdownDescription: "Name of the option",
							Computed:            true,
						},
						"option_description": schema.StringAttribute{
							MarkdownDescription: "Description of the option",
							Computed:            true,
						},
						"permanent": schema.BoolAttribute{
							MarkdownDescription: "Whether the option is permanent",
							Computed:            true,
						},
						"persistent": schema.BoolAttribute{
							MarkdownDescription: "Whether the option is persistent",
							Computed:            true,
						},
						"port": schema.Int64Attribute{
							MarkdownDescription: "Port associated with the option",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *RDSMSSQLDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring RDS SQL Server data source")

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

func (d *RDSMSSQLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RDSMSSQLDataSourceModel

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

	tflog.Debug(ctx, "Getting RDS SQL Server DB instance information", map[string]interface{}{"db_identifier": data.DBIdentifier.ValueString()})

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

	// Check if this is a SQL Server instance
	if *instance.Engine != "sqlserver-ee" && *instance.Engine != "sqlserver-se" && *instance.Engine != "sqlserver-ex" && *instance.Engine != "sqlserver-web" {
		resp.Diagnostics.AddError("Not a SQL Server instance", fmt.Sprintf("The DB instance %s is not a SQL Server instance. Engine: %s", data.DBIdentifier.ValueString(), *instance.Engine))
		return
	}

	// Set ID for Terraform state tracking
	data.ID = types.StringValue(data.DBIdentifier.ValueString())

	// Process Parameter Group information
	if len(instance.DBParameterGroups) > 0 {
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

		if len(pgResp.DBParameterGroups) > 0 {
			data.FamilyName = types.StringValue(*pgResp.DBParameterGroups[0].DBParameterGroupFamily)
			data.PGDescription = types.StringValue(*pgResp.DBParameterGroups[0].Description)
		} else {
			tflog.Warn(ctx, "No parameter group details found", map[string]interface{}{"parameter_group": *instance.DBParameterGroups[0].DBParameterGroupName})
		}
	} else {
		tflog.Warn(ctx, "No parameter groups configured for this instance", map[string]interface{}{"db_identifier": data.DBIdentifier.ValueString()})
	}

	// Process Option Group information
	if len(instance.OptionGroupMemberships) > 0 {
		// Set option group value
		optionGroupName := *instance.OptionGroupMemberships[0].OptionGroupName
		data.OptionGroup = types.StringValue(optionGroupName)

		// Get option group details
		ogInput := &rds.DescribeOptionGroupsInput{
			OptionGroupName: aws.String(optionGroupName),
		}

		ogResp, err := client.DescribeOptionGroups(ctx, ogInput)
		if err != nil {
			resp.Diagnostics.AddError("Failed to describe option groups", fmt.Sprintf("Error describing option groups: %s", err))
			return
		}

		if len(ogResp.OptionGroupsList) > 0 {
			optionGroup := ogResp.OptionGroupsList[0]

			// Set basic option group information
			data.EngineName = types.StringValue(*optionGroup.EngineName)
			data.MajorVersion = types.StringValue(*optionGroup.MajorEngineVersion)
			data.OGDescription = types.StringValue(*optionGroup.OptionGroupDescription)

			// Process options
			options := []MSSQLOptionModel{}
			for _, option := range optionGroup.Options {
				optionModel := MSSQLOptionModel{
					OptionName:        types.StringValue(*option.OptionName),
					OptionDescription: types.StringValue(*option.OptionDescription),
					Permanent:         types.BoolValue(*option.Permanent),
					Persistent:        types.BoolValue(*option.Persistent),
				}

				if option.Port != nil {
					optionModel.Port = types.Int64Value(int64(*option.Port))
				} else {
					optionModel.Port = types.Int64Null()
				}

				options = append(options, optionModel)
			}

			// Convert options to types.List
			optionsList, diags := types.ListValueFrom(ctx, types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"option_name":        types.StringType,
					"option_description": types.StringType,
					"permanent":          types.BoolType,
					"persistent":         types.BoolType,
					"port":               types.Int64Type,
				},
			}, options)

			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}

			data.Options = optionsList
		} else {
			tflog.Warn(ctx, "No option group details found", map[string]interface{}{"option_group": optionGroupName})
		}
	} else {
		tflog.Warn(ctx, "No option groups configured for this instance", map[string]interface{}{"db_identifier": data.DBIdentifier.ValueString()})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
