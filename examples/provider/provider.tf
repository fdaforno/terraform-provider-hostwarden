terraform {
  required_providers {
    freecenter = { source = "fdaforno/freecenter" }
  }
}

# Credentials can also come from the environment:
# FREECENTER_ADDRESS, FREECENTER_USERNAME and FREECENTER_PASSWORD, or FREECENTER_TOKEN.
provider "freecenter" {
  address  = "http://freecenter.example.lan:8080"
  username = "admin"
  password = var.freecenter_password
}
