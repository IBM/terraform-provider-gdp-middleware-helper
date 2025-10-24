provider "gdp-middleware-helper" {
  host = "localhost"
  port = "8080"
}

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