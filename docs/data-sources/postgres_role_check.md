# postgres_role_check Data Source

This data source checks if a specific role exists in a PostgreSQL database.

## Example Usage

```terraform
data "gdp-middleware-helper_postgres_role_check" "example" {
  host      = "localhost"
  port      = "5432"
  username  = "postgres"
  password  = "postgres"
  db_name   = "postgres"
  ssl_mode  = "disable"
  role_name = "my_terraform_role"
}

output "role_exists" {
  value = data.gdp-middleware-helper_postgres_role_check.example.exists
}
```

## Argument Reference

* `host` - (Required) PostgreSQL server hostname or IP address.
* `port` - (Optional) PostgreSQL server port. Defaults to "5432".
* `username` - (Required) PostgreSQL username.
* `password` - (Required) PostgreSQL password.
* `db_name` - (Required) PostgreSQL database name.
* `ssl_mode` - (Optional) PostgreSQL SSL mode (disable, require, verify-ca, verify-full). Defaults to "disable".
* `role_name` - (Required) Name of the PostgreSQL role to check for.

## Attribute Reference

* `exists` - Boolean indicating whether the specified role exists in the PostgreSQL database.