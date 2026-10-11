terraform {
  required_providers {
    hostwarden = { source = "fdaforno/hostwarden" }
  }
}

# Credentials can also come from the environment:
# HOSTWARDEN_ADDRESS, HOSTWARDEN_USERNAME and HOSTWARDEN_PASSWORD, or HOSTWARDEN_TOKEN.
provider "hostwarden" {
  address  = "http://hostwarden.example.lan:8080"
  username = "admin"
  password = var.hostwarden_password
}
