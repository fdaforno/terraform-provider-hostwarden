# Provider configuration shared by the examples in this folder.
#
# Authentication, either:
#   - username + password: the provider opens the session itself and renews it
#     when it expires (FREECENTER_USERNAME / FREECENTER_PASSWORD)
#   - token: a bearer token from POST /api/v1/auth/login (FREECENTER_TOKEN);
#     it expires after 7 days without use
#
# Changing anything requires a user with the admin role.

terraform {
  required_providers {
    freecenter = { source = "registry.terraform.io/fdaforno/freecenter" }
  }
}

provider "freecenter" {
  address  = var.freecenter_address
  username = var.freecenter_username
  password = var.freecenter_password
}

variable "freecenter_address" {
  description = "URL of the FreeCenter server."
  type        = string
  default     = "http://localhost:8080"
}

variable "freecenter_username" {
  description = "FreeCenter user (admin role)."
  type        = string
  default     = "admin"
}

variable "freecenter_password" {
  description = "Password of the FreeCenter user."
  type        = string
  sensitive   = true
}

variable "datastore" {
  description = "Datastore of the first host used by the examples."
  type        = string
  default     = "datastore1"
}

variable "guest_password" {
  description = "Password set inside the example guests by cloud-init."
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "SSH public key authorized inside the example guests."
  type        = string
}

# A registered ESXi host by name, with its version, CPU and status
data "freecenter_host" "main" {
  name = "esxi-01"
}

# A second host: target of the migration and remote backup examples
data "freecenter_host" "second" {
  name = "esxi-02"
}

# All datastores of a host
data "freecenter_datastores" "all" {
  host_id = data.freecenter_host.main.id
}

# Networks available to virtual machines, virtual switches and port groups
data "freecenter_networks" "main" {
  host_id = data.freecenter_host.main.id
}
