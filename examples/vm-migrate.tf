# Migrating a virtual machine between ESXi hosts.
#
# Changing host_id migrates the VM: its files are copied directly between the
# two hosts and removed from the source. datastore names the datastore on the
# target host.
#
# For a running VM FreeCenter picks the mode from the CPUs of the two hosts:
#   - same CPU      -> suspend    (suspend, copy, resume)
#   - different CPU -> stop_start (shut the guest down, copy, power on)
# A suspended VM cannot resume on a different CPU: use stop_start there.
# A powered-off VM is simply moved (cold).

variable "app_host" {
  description = "Host the VM runs on: change it from main to second to migrate it."
  type        = string
  default     = "main"
}

locals {
  hosts = {
    main   = { id = data.freecenter_host.main.id, datastore = var.datastore }
    second = { id = data.freecenter_host.second.id, datastore = "datastore1" }
  }
}

resource "freecenter_vm" "app" {
  host_id   = local.hosts[var.app_host].id
  datastore = local.hosts[var.app_host].datastore

  name     = "app-01"
  guest_id = "ubuntu64Guest"

  num_cpus      = 2
  memory_mb     = 4096
  disk_sizes_gb = [20]
  network       = "VM Network"

  # Migration options (all optional)
  migrate_mode         = "stop_start" # leave it out to let FreeCenter choose
  migrate_precopy      = true         # copy the disks while the VM runs, then stop it: shorter stop
  migrate_reclaim_thin = true         # make the copied disks thin again

  power = "on"
}

# terraform apply -var app_host=second
