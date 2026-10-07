# Snapshots, backups, schedules and users.

# A snapshot (destroying the resource deletes it)
resource "freecenter_vm_snapshot" "before_upgrade" {
  vm_id       = freecenter_vm.app.id
  name        = "before-upgrade"
  description = "Before the upgrade"
  memory      = false
}

# A one-off backup: a consolidated copy of disks and configuration in the
# freecenter-backups folder of the datastore. Works on a running VM too.
resource "freecenter_backup" "golden" {
  vm_id     = freecenter_vm.golden.id
  datastore = var.datastore

  # To move the backup to another host:
  # target_host_id   = data.freecenter_host.second.id
  # target_datastore = "datastore1"

  # Changing a value here takes a new backup
  triggers = { release = "2026-10" }
}

# Restoring a backup as a new VM (on the host that holds it)
resource "freecenter_vm" "golden_restored" {
  host_id      = freecenter_backup.golden.host_id
  name         = "golden-restored"
  datastore    = var.datastore
  restore_from = freecenter_backup.golden.id
  power        = "off"
}

# A snapshot every night, keeping the last 7
resource "freecenter_schedule" "app_nightly_snapshot" {
  vm_id     = freecenter_vm.app.id
  kind      = "snapshot"
  frequency = "daily"
  hour      = 2
  minute    = 30
  timezone  = "Europe/Rome"
  keep      = 7
}

# A weekly backup (Sunday at 03:00) moved to the other host
resource "freecenter_schedule" "app_weekly_backup" {
  vm_id     = freecenter_vm.app.id
  kind      = "backup"
  frequency = "weekly"
  weekday   = 0
  hour      = 3
  timezone  = "Europe/Rome"
  keep      = 4

  backup_datastore        = var.datastore
  backup_target_host_id   = data.freecenter_host.second.id
  backup_target_datastore = "datastore1"
}

# A read-only user (sees everything, changes nothing, opens no consoles)
variable "auditor_password" {
  type      = string
  sensitive = true
}

resource "freecenter_user" "auditor" {
  username = "auditor"
  password = var.auditor_password
  role     = "readonly"
}
