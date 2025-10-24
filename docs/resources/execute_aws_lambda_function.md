# execute_aws_lambda_function Resource

This resource executes an AWS Lambda function and captures the result. It's useful for triggering Lambda functions as part of your Terraform workflow, especially for initialization or configuration tasks.

## Example Usage

```terraform
resource "gdp-middleware-helper_execute_aws_lambda_function" "example" {
  function_name = aws_lambda_function.documentdb_lambda.function_name
  region        = var.aws_region
  payload       = "{}"
  output_path   = "${path.module}/lambda_execution_result.json"
}

output "lambda_result" {
  value = gdp-middleware-helper_execute_aws_lambda_function.example.execution_result
}
```

## Argument Reference

* `function_name` - (Required) The name of the AWS Lambda function to execute.
* `region` - (Required) The AWS region where the Lambda function is deployed.
* `payload` - (Optional) JSON payload to send to the Lambda function. Defaults to `{}`.
* `output_path` - (Optional) Path where the Lambda execution result will be saved. Defaults to `lambda_execution_result.json`.

## Attribute Reference

* `id` - The identifier of the resource (same as `function_name`).
* `execution_result` - The result of the Lambda function execution, including both the AWS SDK output and the parsed JSON result.
* `execution_succeeded` - Boolean indicating whether the Lambda function execution succeeded.

## Import

This resource can be imported using the function name:

```
terraform import gdp-middleware-helper_execute_aws_lambda_function.example function_name