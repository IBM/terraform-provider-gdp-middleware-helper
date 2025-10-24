package provider

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &DocDBParameterGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &DocDBParameterGroupDataSource{}

func NewDocDBParameterGroupDataSource() datasource.DataSource {
	return &DocDBParameterGroupDataSource{}
}

// DocDBParameterGroupDataSource defines the data source implementation.
type DocDBParameterGroupDataSource struct {
	client *docdb.Client
}

// DocDBParameterGroupDataSourceModel describes the data source data model.
type DocDBParameterGroupDataSourceModel struct {
	ClusterIdentifier types.String `tfsdk:"cluster_identifier"`
	Region            types.String `tfsdk:"region"`
	ParameterGroup    types.String `tfsdk:"parameter_group"`
	FamilyName        types.String `tfsdk:"family_name"`
	Description       types.String `tfsdk:"description"`
	ID                types.String `tfsdk:"id"`
}

func (d *DocDBParameterGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_docdb_parameter_group"
}

func (d *DocDBParameterGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Data source for AWS DocumentDB parameter group",

		Attributes: map[string]schema.Attribute{
			"cluster_identifier": schema.StringAttribute{
				MarkdownDescription: "DocumentDB cluster identifier",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "AWS region",
				Optional:            true,
			},
			"parameter_group": schema.StringAttribute{
				MarkdownDescription: "DocumentDB parameter group name",
				Computed:            true,
			},
			"family_name": schema.StringAttribute{
				MarkdownDescription: "DocumentDB family name",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "DocumentDB family name",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source",
			},
		},
	}
}

func (d *DocDBParameterGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring DocumentDB parameter group data source")

	// If provider is not configured, return
	if req.ProviderData == nil {
		return
	}

	// Create AWS config and DocumentDB client
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config: %s", err))
		return
	}

	d.client = docdb.NewFromConfig(awsCfg)
}

func (d *DocDBParameterGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DocDBParameterGroupDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If region is specified, update the AWS config
	var client *docdb.Client
	if !data.Region.IsNull() {
		tflog.Debug(ctx, "configuring client with region")
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(data.Region.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to load AWS SDK config", fmt.Sprintf("Unable to load AWS SDK config with region %s: %s", data.Region.ValueString(), err))
			return
		}
		client = docdb.NewFromConfig(awsCfg)
	} else {
		tflog.Debug(ctx, "using default client")
		client = d.client
	}

	// Get DocumentDB cluster information
	input := &docdb.DescribeDBClustersInput{DBClusterIdentifier: aws.String(data.ClusterIdentifier.ValueString())}

	tflog.Debug(ctx, "Getting DocumentDB cluster information", map[string]interface{}{"cluster_identifier": data.ClusterIdentifier.ValueString()})

	result, err := client.DescribeDBClusters(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to describe DocumentDB clusters", fmt.Sprintf("Error describing DocumentDB clusters: %s", err))
		return
	}

	if len(result.DBClusters) == 0 {
		resp.Diagnostics.AddError("DocumentDB cluster not found", fmt.Sprintf("No DocumentDB cluster found with identifier: %s", data.ClusterIdentifier.ValueString()))
		return
	}

	cluster := result.DBClusters[0]

	// Set parameter group value
	data.ParameterGroup = types.StringValue(*cluster.DBClusterParameterGroup)

	// Now, describe the parameter group to get the family
	pgInput := &docdb.DescribeDBClusterParameterGroupsInput{
		DBClusterParameterGroupName: cluster.DBClusterParameterGroup,
	}

	pgResp, err := client.DescribeDBClusterParameterGroups(ctx, pgInput)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("failed to describe DB cluster parameter groups, %v", err), fmt.Sprintf("failed to describe DB cluster parameter groups, %v", err))
		return
	}

	if len(pgResp.DBClusterParameterGroups) == 0 {
		resp.Diagnostics.AddError(fmt.Sprintf("no family group configured for document db cluster %s", data.ClusterIdentifier.ValueString()), fmt.Sprintf("no family group configured for document db cluster %s", data.ClusterIdentifier.ValueString()))
		return
	}
	
	data.FamilyName = types.StringValue(*pgResp.DBClusterParameterGroups[0].DBParameterGroupFamily)
	data.Description = types.StringValue(*pgResp.DBClusterParameterGroups[0].Description)
	data.ID = types.StringValue(data.ClusterIdentifier.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
