# Configuration of an ESXi host.

# Registering a host (Hostwarden stores the password encrypted).
#
# resource "hostwarden_host" "lab" {
#   name     = "esxi-lab"
#   address  = "192.168.1.20"
#   username = "root"
#   password = var.esxi_lab_password
#
#   # maintenance_mode = true   # fails while virtual machines are running
# }

# An internal standard virtual switch (no uplinks) and a port group with a VLAN
resource "hostwarden_vswitch" "lab" {
  host_id = data.hostwarden_host.main.id
  name    = "vSwitch-lab"
  # uplinks = ["vmnic2"]   # physical adapters not used by another switch
}

resource "hostwarden_port_group" "lab" {
  host_id = data.hostwarden_host.main.id
  name    = "lab"
  vswitch = hostwarden_vswitch.lab.name
  vlan    = 120 # can be changed in place
}

# NTP: keep the host clock synchronized
resource "hostwarden_host_ntp" "main" {
  host_id = data.hostwarden_host.main.id
  servers = ["0.pool.ntp.org", "1.pool.ntp.org"]
}

# Host services: state and startup policy (here the ESXi Shell)
resource "hostwarden_host_service" "shell" {
  host_id = data.hostwarden_host.main.id
  key     = "TSM"
  running = false
  policy  = "off"
}
