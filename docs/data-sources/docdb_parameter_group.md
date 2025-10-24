---
page_title: "gdp-middleware-helper_docdb_parameter_group Data Source"
subcategory: ""
description: |-
  This data source retrieves information about an AWS DocumentDB parameter group
---

# docdb_parameter_group Data Source

This data source retrieves information about an AWS DocumentDB parameter group associated with a specific DocumentDB cluster.

## Example Usage

```terraform
data "gdp-middleware-helper_docdb_parameter_group" "example" {
  cluster_identifier = "my-docdb-cluster"
  region             = "us-east-1"
}

output "parameter_group_name" {
  value = data.gdp-middleware-helper_docdb_parameter_group.example.parameter_group
}

output "family_name" {
  value = data.gdp-middleware-helper_docdb_parameter_group.example.family_name
}
```

## Argument Reference

* `cluster_identifier` - (Required) The identifier of the DocumentDB cluster.
* `region` - (Optional) The AWS region where the DocumentDB cluster is located. If not specified, the provider's default region is used.

## Attribute Reference

* `id` - The identifier of the data source (same as `cluster_identifier`).
* `parameter_group` - The name of the DocumentDB parameter group associated with the cluster.
* `family_name` - The family name of the DocumentDB parameter group.
* `description` - The description of the DocumentDB parameter group.