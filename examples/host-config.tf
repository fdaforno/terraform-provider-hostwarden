# Configuration of an ESXi host.

# Registering a host (FreeCenter stores the password encrypted).
#
# resource "freecenter_host" "lab" {
#   name     = "esxi-lab"
#   address  = "192.168.1.20"
#   username = "root"
#   password = var.esxi_lab_password
#
#   # maintenance_mode = true   # fails while virtual machines are running
# }

# An internal standard virtual switch (no uplinks) and a port group with a VLAN
resource "freecenter_vswitch" "lab" {
  host_id = data.freecenter_host.main.id
  name    = "vSwitch-lab"
  # uplinks = ["vmnic2"]   # physical adapters not used by another switch
}

resource "freecenter_port_group" "lab" {
  host_id = data.freecenter_host.main.id
  name    = "lab"
  vswitch = freecenter_vswitch.lab.name
  vlan    = 120 # can be changed in place
}

# NTP: keep the host clock synchronized
resource "freecenter_host_ntp" "main" {
  host_id = data.freecenter_host.main.id
  servers = ["0.pool.ntp.org", "1.pool.ntp.org"]
}

# Host services: state and startup policy (here the ESXi Shell)
resource "freecenter_host_service" "shell" {
  host_id = data.freecenter_host.main.id
  key     = "TSM"
  running = false
  policy  = "off"
}
