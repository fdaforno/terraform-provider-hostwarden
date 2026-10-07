# A single Ubuntu server with cloud-init: SSH key and nginx.
#
# Requires the Ubuntu 24.04 cloud image on the datastore, as
# cloud-images/ubuntu-24.04.vmdk (download it from the New Virtual Machine
# wizard of FreeCenter).

resource "freecenter_vm" "ubuntu" {
  host_id  = data.freecenter_host.main.id
  name     = "ubuntu-web"
  guest_id = "ubuntu64Guest"

  datastore = var.datastore
  num_cpus  = 2
  memory_mb = 4096

  network      = "VM Network"
  adapter_type = "vmxnet3"

  annotation = "Ubuntu 24.04 with nginx"

  cloud_image = "cloud-images/ubuntu-24.04.vmdk"

  cloud_init {
    hostname            = "ubuntu-web"
    ssh_authorized_keys = [var.ssh_public_key]

    packages = ["nginx", "vim", "curl"]

    runcmd = [
      "systemctl enable --now nginx",
      "echo '<h1>Hello from FreeCenter!</h1>' > /var/www/html/index.html",
    ]
  }

  power = "on"
}
