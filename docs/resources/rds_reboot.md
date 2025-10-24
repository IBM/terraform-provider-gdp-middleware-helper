---
page_title: "gdp-middleware-helper_rds_reboot Resource"
subcategory: ""
description: |-
  Resource for rebooting an AWS RDS instance
---

# rds_reboot Resource

This resource allows you to reboot an AWS RDS instance as part of your Terraform workflow. It's useful for applying parameter group changes or performing maintenance operations that require a reboot.

## Example Usage

```terraform
resource "gdp-middleware-helper_rds_reboot" "example" {
  db_instance_identifier = aws_db_instance.example.identifier
  region                 = "us-east-1"
  force_failover         = false
}

output "last_reboot_time" {
  value = gdp-middleware-helper_rds_reboot.example.last_reboot_time
}
```

## Argument Reference

* `db_instance_identifier` - (Required) The identifier of the RDS instance to reboot. Changing this forces a new resource to be created.
* `region` - (Optional) The AWS region where the RDS instance is located. If not specified, the provider's default region is used.
* `force_failover` - (Optional) When true, the reboot is conducted through a MultiAZ failover. Default is false.

## Attribute Reference

* `id` - The identifier of the resource (same as `db_instance_identifier`).
* `last_reboot_time` - Timestamp of the last reboot operation.

## Import

This resource can be imported using the DB instance identifier:

```
terraform import gdp-middleware-helper_rds_reboot.example my-db-instance