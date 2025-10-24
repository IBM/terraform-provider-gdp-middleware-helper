---
page_title: "Guardium Data Protection Middleware Helper Provider"
subcategory: ""
description: |-
  The Guardium Data Protection Middleware Helper provider is used to interact with various middleware services.
---

# Guardium Data Protection Provider

The Guardium Data Protection provider is used to interact with various middleware services. It provides resources and data sources for managing and querying AWS services like RDS, DocumentDB, and Lambda.

## Example Usage

```terraform
terraform {
  required_providers {
    gdp-middleware-helper = {
      source = "guardium-data-protection/gdp-middleware-helper"
      version = "~> 1.0"
    }
  }
}

provider "gdp-middleware-helper" {
  # Configuration options
}

# Example: Query a DocumentDB parameter group
data "gdp-middleware-helper_docdb_parameter_group" "example" {
  cluster_identifier = "my-docdb-cluster"
  region             = "us-east-1"
}
```

## Data Sources

The provider includes the following data sources:

* [docdb_parameter_group](data-sources/docdb_parameter_group.md) - Retrieve information about an AWS DocumentDB parameter group
* [postgres_role_check](data-sources/postgres_role_check.md) - Check if a specific role exists in a PostgreSQL database
* [rds_mariadb](data-sources/rds_mariadb.md) - Retrieve information about an AWS RDS MariaDB instance's parameter and option groups
* [rds_postgres_parameter_group](data-sources/rds_postgres_parameter_group.md) - Retrieve information about an AWS RDS PostgreSQL parameter group

## Resources

The provider includes the following resources:

* [execute_aws_lambda_function](resources/execute_aws_lambda_function.md) - Execute an AWS Lambda function and capture the result
* [rds_reboot](resources/rds_reboot.md) - Reboot an AWS RDS instance