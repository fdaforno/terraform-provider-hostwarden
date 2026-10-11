# Provider configuration shared by the examples in this folder.
#
# Authentication, either:
#   - username + password: the provider opens the session itself and renews it
#     when it expires (HOSTWARDEN_USERNAME / HOSTWARDEN_PASSWORD)
#   - token: a bearer token from POST /api/v1/auth/login (HOSTWARDEN_TOKEN);
#     it expires after 7 days without use
#
# Changing anything requires a user with the admin role.

terraform {
  required_providers {
    hostwarden = { source = "registry.terraform.io/fdaforno/hostwarden" }
  }
}

provider "hostwarden" {
  address  = var.hostwarden_address
  username = var.hostwarden_username
  password = var.hostwarden_password
}

variable "hostwarden_address" {
  description = "URL of the Hostwarden server."
  type        = string
  default     = "http://localhost:8080"
}

variable "hostwarden_username" {
  description = "Hostwarden user (admin role)."
  type        = string
  default     = "admin"
}

variable "hostwarden_password" {
  description = "Password of the Hostwarden user."
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
data "hostwarden_host" "main" {
  name = "esxi-01"
}

# A second host: target of the migration and remote backup examples
data "hostwarden_host" "second" {
  name = "esxi-02"
}

# All datastores of a host
data "hostwarden_datastores" "all" {
  host_id = data.hostwarden_host.main.id
}

# Networks available to virtual machines, virtual switches and port groups
data "hostwarden_networks" "main" {
  host_id = data.hostwarden_host.main.id
}
