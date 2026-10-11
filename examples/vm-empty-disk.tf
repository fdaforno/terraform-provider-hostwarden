# An empty virtual machine: no cloud image, no ISO.
# Two thin disks of 20 and 50 GB; left powered off.

resource "hostwarden_vm" "empty" {
  host_id  = data.hostwarden_host.main.id
  name     = "test-empty"
  guest_id = "rockylinux_64Guest"

  datastore = var.datastore
  num_cpus  = 1
  memory_mb = 2048

  network      = "VM Network"
  adapter_type = "vmxnet3"

  disk_sizes_gb = [20, 50]
  disk_thin     = true

  annotation = "Empty disks: install the guest OS yourself"

  power = "off"
}
