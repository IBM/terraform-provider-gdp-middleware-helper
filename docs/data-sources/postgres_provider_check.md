# postgres_provider_check Data Source

This data source checks if a specific PostgreSQL provider/extension exists in a PostgreSQL database.

## Example Usage

```terraform
data "gdp-middleware-helper_postgres_provider_check" "example" {
  host          = "localhost"
  port          = "5432"
  username      = "postgres"
  password      = "postgres"
  database_name = "postgres"
  ssl_mode      = "disable"
  provider_name = "postgis"
}

output "provider_exists" {
  value = data.gdp-middleware-helper_postgres_provider_check.example.exists
}
```

## Argument Reference

* `host` - (Required) PostgreSQL server hostname or IP address.
* `port` - (Optional) PostgreSQL server port. Defaults to "5432".
* `username` - (Required) PostgreSQL username.
* `password` - (Required) PostgreSQL password.
* `database_name` - (Required) PostgreSQL database name.
* `ssl_mode` - (Optional) PostgreSQL SSL mode (disable, require, verify-ca, verify-full). Defaults to "disable".
* `provider_name` - (Required) Name of the PostgreSQL provider/extension to check for.

## Attribute Reference

* `exists` - Boolean indicating whether the specified provider exists in the PostgreSQL database.