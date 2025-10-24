---
page_title: "gdp-middleware-helper_rds_postgres_parameter_group Data Source"
subcategory: ""
description: |-
  This data source retrieves information about an AWS RDS PostgreSQL parameter group
---

# rds_postgres_parameter_group Data Source

This data source retrieves information about an AWS RDS PostgreSQL parameter group associated with a specific PostgreSQL instance.

## Example Usage

```terraform
data "gdp-middleware-helper_rds_postgres_parameter_group" "example" {
  db_identifier = "my-postgres-instance"
  region        = "us-east-1"
}

output "parameter_group_name" {
  value = data.gdp-middleware-helper_rds_postgres_parameter_group.example.parameter_group
}

output "family_name" {
  value = data.gdp-middleware-helper_rds_postgres_parameter_group.example.family_name
}
```

## Argument Reference

* `db_identifier` - (Required) The identifier of the RDS PostgreSQL instance.
* `region` - (Optional) The AWS region where the RDS PostgreSQL instance is located. If not specified, the provider's default region is used.

## Attribute Reference

* `id` - The identifier of the data source (same as `db_identifier`).
* `parameter_group` - The name of the parameter group associated with the RDS PostgreSQL instance.
* `family_name` - The family name of the parameter group.
* `description` - The description of the parameter group.