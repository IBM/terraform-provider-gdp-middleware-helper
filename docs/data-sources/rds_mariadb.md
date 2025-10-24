---
page_title: "gdp-middleware-helper_rds_mariadb Data Source"
subcategory: ""
description: |-
  This data source retrieves information about an AWS RDS MariaDB instance's parameter and option groups
---

# rds_mariadb Data Source

This data source retrieves information about an AWS RDS MariaDB instance's parameter and option groups.

## Example Usage

```terraform
data "gdp-middleware-helper_rds_mariadb" "example" {
  db_identifier = "my-mariadb-instance"
  region        = "us-east-1"
}

output "parameter_group_name" {
  value = data.gdp-middleware-helper_rds_mariadb.example.parameter_group
}

output "option_group_name" {
  value = data.gdp-middleware-helper_rds_mariadb.example.option_group
}

output "available_options" {
  value = data.gdp-middleware-helper_rds_mariadb.example.options
}
```

## Argument Reference

* `db_identifier` - (Required) The identifier of the RDS MariaDB instance.
* `region` - (Optional) The AWS region where the RDS MariaDB instance is located. If not specified, the provider's default region is used.

## Attribute Reference

* `id` - The identifier of the data source (same as `db_identifier`).

### Parameter Group Attributes

* `parameter_group` - The name of the parameter group associated with the RDS MariaDB instance.
* `family_name` - The family name of the parameter group.
* `parameter_group_description` - The description of the parameter group.

### Option Group Attributes

* `option_group` - The name of the option group associated with the RDS MariaDB instance.
* `engine_name` - The database engine name.
* `major_version` - The major version of the database engine.
* `option_group_description` - The description of the option group.
* `options` - A list of options enabled in the option group. Each option contains:
  * `option_name` - The name of the option.
  * `option_description` - The description of the option.
  * `permanent` - Whether the option is permanent.
  * `persistent` - Whether the option is persistent.
  * `port` - The port associated with the option, if applicable.