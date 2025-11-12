// Copyright (c) IBM Corporation
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure GDPMiddlewareHelperProvider satisfies various provider interfaces.
var _ provider.Provider = &GDPMiddlewareHelperProvider{}

// GDPMiddlewareHelperProvider defines the provider implementation.
type GDPMiddlewareHelperProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

type gdpMiddlewareHelperModel struct{}

func (p *GDPMiddlewareHelperProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "gdp-middleware-helper"
	resp.Version = p.version
}

func (p *GDPMiddlewareHelperProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The GDP Middleware Helper provider is used to interact with various middleware services.",
		Attributes:  map[string]schema.Attribute{},
	}
}

// Configure takes in the defined parameters in the TF module
func (p *GDPMiddlewareHelperProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "configuring provider")
	var data gdpMiddlewareHelperModel
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// No configuration needed for this provider
	resp.DataSourceData = struct{}{}
	resp.ResourceData = struct{}{}
	tflog.Info(ctx, "provider configuration complete")
}

func (p *GDPMiddlewareHelperProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewExecuteAwsLambdaFunctionResource,
		NewRDSRebootResource,
		NewRDSModifyResource,
	}
}

func (p *GDPMiddlewareHelperProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPostgresRoleCheckDataSource,
		NewDocDBParameterGroupDataSource,
		NewRDSPostgresParameterGroupDataSource,
		NewRDSMariaDBDataSource,
		NewRDSMySQLDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &GDPMiddlewareHelperProvider{
			version: version,
		}
	}
}
