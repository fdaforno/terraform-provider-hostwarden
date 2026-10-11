# Templates and clones.
#
# A template is a powered-off virtual machine marked as such: it cannot be
# powered on and is the source of clone_from. A clone is created on the host
# of its source; the other arguments (CPU, memory, networks, firmware...) are
# applied to the clone before its first boot.

resource "hostwarden_vm" "golden" {
  host_id  = data.hostwarden_host.main.id
  name     = "golden-linux"
  guest_id = "otherLinux64Guest"

  datastore     = var.datastore
  num_cpus      = 1
  memory_mb     = 1024
  disk_sizes_gb = [16]
  firmware      = "efi"

  template = true # power becomes "off" by itself
}

resource "hostwarden_vm" "worker" {
  count = 2

  host_id    = data.hostwarden_host.main.id
  name       = "worker-${count.index + 1}"
  datastore  = var.datastore
  clone_from = hostwarden_vm.golden.id

  num_cpus  = 2
  memory_mb = 4096

  network             = "VM Network"
  additional_networks = [hostwarden_port_group.lab.name] # a second network adapter
  adapter_type        = "vmxnet3"

  nested_hv      = true # nested virtualization
  cpu_hot_add    = true
  memory_hot_add = true

  power = "on"
}

# An existing template (made in the web console) is found by name:
#
# data "hostwarden_vm" "tpl" {
#   host_id = data.hostwarden_host.main.id
#   name    = "rocky9-template"
# }
#
# ... clone_from = data.hostwarden_vm.tpl.id
