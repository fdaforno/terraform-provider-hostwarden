# Three Rocky Linux servers provisioned by cloud-init.
#
# Requirements:
#   1. The Rocky 9 cloud image on the datastore, as cloud-images/rocky-9.vmdk:
#      download it from the New Virtual Machine wizard of FreeCenter
#      ("Deploy from a cloud image").
#   2. A DHCP server on the "VM Network" port group.
#
# On first boot cloud-init sets the host name and the root password,
# authorizes the SSH key, installs the packages and runs the commands.

resource "freecenter_vm" "rocky" {
  count   = 3
  host_id = data.freecenter_host.main.id

  name      = "rocky-${count.index + 1}"
  guest_id  = "rockylinux_64Guest"
  datastore = var.datastore
  num_cpus  = 2
  memory_mb = 4096

  network      = "VM Network"
  adapter_type = "vmxnet3"

  annotation = "Rocky Linux provisioned by Terraform and cloud-init"

  cloud_image = "cloud-images/rocky-9.vmdk"

  cloud_init {
    hostname            = "rocky-${count.index + 1}"
    root_password       = var.guest_password
    ssh_authorized_keys = [var.ssh_public_key]

    packages = ["vim", "curl", "wget", "net-tools", "tcpdump", "bind-utils"]

    runcmd = [
      "echo 'Provisioned by Terraform' > /etc/motd",
      "systemctl enable --now sshd",
    ]
  }

  power = "on"
}

output "vm_ids" {
  value       = freecenter_vm.rocky[*].id
  description = "FreeCenter id (<host_id>/<vm_id>) of each VM."
}

output "vm_names" {
  value = freecenter_vm.rocky[*].name
}
