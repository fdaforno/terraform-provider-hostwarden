# A virtual machine that boots from an ISO for a manual installation.
# The ISO must already be on the datastore: upload it, or download it by URL,
# from the datastore browser of FreeCenter. Then follow the installer in the
# web console of the VM.

resource "freecenter_vm" "manual" {
  host_id  = data.freecenter_host.main.id
  name     = "manual-install"
  guest_id = "rockylinux_64Guest"

  datastore = var.datastore
  num_cpus  = 2
  memory_mb = 4096

  network      = "VM Network"
  adapter_type = "vmxnet3"

  disk_sizes_gb = [20]
  disk_thin     = true

  iso_path        = "[${var.datastore}] ISO/Rocky-9-latest-x86_64-boot.iso"
  boot_from_cdrom = true

  annotation = "Boots from the installer ISO"

  power = "on"
}
