provider "gdp-middleware-helper" {}

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