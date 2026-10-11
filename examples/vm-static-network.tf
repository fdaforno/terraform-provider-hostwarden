# A virtual machine with a static address (no DHCP), set on first boot by
# cloud-init through NetworkManager.

resource "hostwarden_vm" "static" {
  host_id  = data.hostwarden_host.main.id
  name     = "rocky-static"
  guest_id = "rockylinux_64Guest"

  datastore = var.datastore
  num_cpus  = 1
  memory_mb = 2048

  network      = "VM Network"
  adapter_type = "vmxnet3"

  cloud_image = "cloud-images/rocky-9.vmdk"

  cloud_init {
    hostname            = "rocky-static"
    ssh_authorized_keys = [var.ssh_public_key]

    # The Rocky cloud image names its connection "System eth0".
    runcmd = [
      "nmcli connection modify 'System eth0' ipv4.method manual ipv4.addresses 192.168.1.50/24 ipv4.gateway 192.168.1.1 ipv4.dns '192.168.1.1 9.9.9.9'",
      "nmcli connection up 'System eth0'",
    ]
  }

  power = "on"
}
