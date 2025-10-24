package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	lambdasvc "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &executeAwsLambdaFunctionResource{}
var _ resource.ResourceWithImportState = &executeAwsLambdaFunctionResource{}

// NewExecuteAwsLambdaFunctionResource is a helper function to simplify the provider implementation.
func NewExecuteAwsLambdaFunctionResource() resource.Resource {
	return &executeAwsLambdaFunctionResource{}
}

// executeAwsLambdaFunctionResource is the resource implementation.
type executeAwsLambdaFunctionResource struct{}

// executeAwsLambdaFunctionResourceModel maps the resource schema data.
type executeAwsLambdaFunctionResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	FunctionName       types.String `tfsdk:"function_name"`
	Region             types.String `tfsdk:"region"`
	SourceCodeHash     types.String `tfsdk:"source_code_hash"`
	ExecutionSucceeded types.Bool   `tfsdk:"execution_succeeded"`
}

// Metadata returns the resource type name.
func (r *executeAwsLambdaFunctionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_execute_aws_lambda_function"
}

// Schema defines the schema for the resource.
func (r *executeAwsLambdaFunctionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Executes an AWS Lambda function and returns the result.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this resource. Set to function name.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"function_name": schema.StringAttribute{
				Description: "Name of the AWS Lambda function to execute.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "AWS region where the Lambda function is deployed.",
				Required:    true,
			},
			"source_code_hash": schema.StringAttribute{
				Description: "The hash of the lambda zip that is being uploaded",
				Required:    true,
			},
			"execution_succeeded": schema.BoolAttribute{
				Description: "Whether the Lambda function execution succeeded.",
				Computed:    true,
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *executeAwsLambdaFunctionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan executeAwsLambdaFunctionResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Execute the AWS Lambda function
	success, err := executeLambdaFunction(
		ctx,
		plan.FunctionName.ValueString(),
		plan.Region.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to execute AWS Lambda function",
			fmt.Sprintf("Failed to execute AWS Lambda function: %s", err),
		)
		return
	}

	// Set resource ID to function name
	plan.ID = plan.FunctionName
	plan.ExecutionSucceeded = types.BoolValue(success)

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *executeAwsLambdaFunctionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state executeAwsLambdaFunctionResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *executeAwsLambdaFunctionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan executeAwsLambdaFunctionResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Execute the AWS Lambda function
	success, err := executeLambdaFunction(
		ctx,
		plan.FunctionName.ValueString(),
		plan.Region.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to execute AWS Lambda function",
			fmt.Sprintf("Failed to execute AWS Lambda function: %s", err),
		)
		return
	}

	plan.ExecutionSucceeded = types.BoolValue(success)

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *executeAwsLambdaFunctionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state executeAwsLambdaFunctionResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// ImportState imports the resource into Terraform state.
func (r *executeAwsLambdaFunctionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type lambdaResultPayload struct {
	StatusCode int `json:"statusCode"`
}

// executeLambdaFunction executes the AWS Lambda function using the AWS SDK and returns the result.
func executeLambdaFunction(ctx context.Context, functionName, region string) (bool, error) {
	tflog.Info(ctx, fmt.Sprintf("Invoking Lambda function %s...", functionName))

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Error loading AWS config: %s", err))
		return false, err
	}

	// Create Lambda client
	lambdaClient := lambdasvc.NewFromConfig(cfg)

	// Prepare the invoke input
	input := &lambdasvc.InvokeInput{
		FunctionName: aws.String(functionName),
	}

	// Invoke the Lambda function
	result, err := lambdaClient.Invoke(ctx, input)
	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Error invoking Lambda function: %s", err))
		return false, err
	}

	// Process the response
	responsePayload := string(result.Payload)

	lrp := new(lambdaResultPayload)
	if err = json.Unmarshal([]byte(responsePayload), lrp); err != nil {
		tflog.Error(ctx, fmt.Sprintf("failed to parse payload: %s", err))
		return false, err
	}

	// Format the result for output
	var formattedResult strings.Builder
	formattedResult.WriteString(fmt.Sprintf("StatusCode: %d\n", lrp.StatusCode))

	if result.FunctionError != nil {
		formattedResult.WriteString(fmt.Sprintf("FunctionError: %s\n", *result.FunctionError))
	}

	formattedResult.WriteString(fmt.Sprintf("ExecutedVersion: %s\n", aws.ToString(result.ExecutedVersion)))
	formattedResult.WriteString(fmt.Sprintf("Payload: %s\n", responsePayload))
	formattedResult.WriteString(fmt.Sprintf("\nFormatted Payload:\n%s\n", string(result.Payload)))

	// Check if the function executed successfully
	success := lrp.StatusCode >= 200 && lrp.StatusCode < 300 && result.FunctionError == nil

	tflog.Info(ctx, fmt.Sprintf("Lambda execution completed with status code %d", lrp.StatusCode))

	tflog.Debug(ctx, formattedResult.String())
	if !success {
		tflog.Error(ctx, "Lambda execution failed")
		return false, fmt.Errorf("lambda execution failed with status code %d", lrp.StatusCode)
	}

	return success, nil
}
