# Terraform Provider for FreeCenter

Manage **standalone VMware ESXi** hosts with Terraform, through FreeCenter.

This provider talks to [FreeCenter](https://github.com/fdaforno/freecenter), an independent management server that puts a REST API in front of your standalone ESXi hosts. With it you can describe virtual machines, templates, networks, snapshots, backups and schedules as code, and even migrate a virtual machine to another host by changing one attribute.

FreeCenter is a simple interface over the standard administration commands already present on an ESXi host. Its purpose is not to work around the licensing of vCenter Server or of any other product: the one practical advantage is infrastructure as code in a home lab, to build throwaway test systems quickly. It is meant for home labs and test environments, not for production.

You need a running FreeCenter with at least one registered host: see the [FreeCenter README](https://github.com/fdaforno/freecenter#getting-started).

## Example

```hcl
terraform {
  required_providers {
    freecenter = { source = "fdaforno/freecenter" }
  }
}

provider "freecenter" {
  address  = "http://freecenter.example.lan:8080"
  username = "admin"
  password = var.freecenter_password
}

data "freecenter_host" "main" {
  name = "esxi-01"
}

# A port group with a VLAN
resource "freecenter_port_group" "lab" {
  host_id = data.freecenter_host.main.id
  name    = "lab"
  vswitch = "vSwitch0"
  vlan    = 120
}

# A Linux server deployed from a cloud image and configured by cloud-init
resource "freecenter_vm" "web" {
  host_id   = data.freecenter_host.main.id
  datastore = "datastore1"
  name      = "web-01"
  guest_id  = "rockylinux_64Guest"

  cloud_image = "cloud-images/rocky-9.vmdk"
  num_cpus    = 2
  memory_mb   = 4096
  network     = freecenter_port_group.lab.name

  cloud_init {
    hostname            = "web-01"
    ssh_authorized_keys = [file("~/.ssh/id_ed25519.pub")]
    packages            = ["nginx"]
  }
}

# A snapshot every night, keeping the last seven
resource "freecenter_schedule" "web_nightly" {
  vm_id     = freecenter_vm.web.id
  kind      = "snapshot"
  frequency = "daily"
  hour      = 2
  timezone  = "Europe/Rome"
  keep      = 7
}
```

More examples (templates and clones, migration, backups, host configuration, a Kubernetes cluster) are in the [`examples`](examples/) folder.

## Authentication

| Provider argument | Environment variable | |
|---|---|---|
| `address` | `FREECENTER_ADDRESS` | base URL of the FreeCenter server |
| `username`, `password` | `FREECENTER_USERNAME`, `FREECENTER_PASSWORD` | the provider opens the session itself and renews it when it expires |
| `token` | `FREECENTER_TOKEN` | alternative: a token from `POST /api/v1/auth/login` (expires after 7 days without use) |

Changing anything requires a user with the `admin` role.

## Resources

| Resource | Manages |
|---|---|
| `freecenter_host` | registration of an ESXi host in FreeCenter, maintenance mode |
| `freecenter_vm` | a virtual machine: hardware, options, network adapters, templates, cloning, restore, migration |
| `freecenter_vm_snapshot` | a snapshot of a virtual machine |
| `freecenter_backup` | a one-off backup of a virtual machine, optionally moved to another host |
| `freecenter_schedule` | scheduled snapshots or backups with retention |
| `freecenter_port_group` | a port group (with VLAN) on a standard virtual switch |
| `freecenter_vswitch` | a standard virtual switch |
| `freecenter_host_service` | state and startup policy of a host service |
| `freecenter_host_ntp` | NTP configuration of a host |
| `freecenter_user` | a FreeCenter user (`admin` or `readonly`) |

## Data sources

| Data source | Returns |
|---|---|
| `freecenter_host` | a host by name, with version, CPU model and status |
| `freecenter_hosts` | all registered hosts |
| `freecenter_datastores` | datastores of a host with capacity and free space |
| `freecenter_networks` | networks, port groups and virtual switches of a host |
| `freecenter_vm` | a virtual machine or template by name (for example to clone it) |
| `freecenter_backups` | existing backups, optionally of one machine or on one host |

Every argument is documented in [`docs/`](docs/index.md), the pages shown on the Terraform Registry.

## How `freecenter_vm` behaves

- **Changes in place.** FreeCenter applies configuration changes to a powered-off machine. The provider powers it off, applies all changes at once and restores the desired `power` state. Name, guest OS, CPU, memory, networks, firmware and options change in place. Disks can grow and be appended, never shrunk or removed.
- **Migration.** Changing `host_id` (with `datastore` naming the datastore on the target host) migrates the machine: its files are copied directly between the two hosts and removed from the source, and the resource `id` changes. `migrate_mode` chooses `stop_start` or `suspend` (same CPU on both hosts only); when omitted FreeCenter decides from the power state and the CPUs. `migrate_precopy = true` copies the disks while the machine runs, to shorten the stop. Changing `datastore` alone replaces the machine.
- **Clone and restore.** `clone_from` (the id of a powered-off machine or template on the same host) and `restore_from` (the id of a `freecenter_backup`) create the machine from a copy; the other arguments are then applied to it.
- **Templates.** `template = true` marks the machine as a template, which stays powered off.
- **Import.** `terraform import freecenter_vm.web <host_id>/<vm_id>`.

`freecenter_host_service` and `freecenter_host_ntp` describe configuration that exists on the host regardless: destroying them leaves it as it is.

## Building and using a local build

```bash
make build      # vet, test and build ./terraform-provider-freecenter
make docs       # regenerate docs/ after changing a schema
```

Point Terraform at the binary with a CLI configuration file and `TF_CLI_CONFIG_FILE`:

```hcl
# dev.tfrc
provider_installation {
  dev_overrides {
    "registry.terraform.io/fdaforno/freecenter" = "/path/to/terraform-provider-freecenter"
  }
  direct {}
}
```

```bash
export TF_CLI_CONFIG_FILE=$PWD/dev.tfrc
terraform plan      # no terraform init needed with a dev override
```

## Releasing

Pushing a version tag (`v1.2.3`) runs the release workflow: GoReleaser builds the binaries for every platform and signs the checksums with the GPG key stored in the repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`. The Terraform Registry picks the release up from GitHub.

## Compatibility

The provider follows the REST API of FreeCenter. Use a provider release with a FreeCenter of the same period or newer; when the API gains something the provider needs, the release notes say which FreeCenter version is required.

## License

[Apache License 2.0](LICENSE).

## Disclaimer

An independent project: not affiliated with, endorsed, sponsored or supported by Broadcom, VMware or HashiCorp. VMware, ESXi, vSphere and vCenter are registered trademarks or trademarks of Broadcom Inc. and/or its subsidiaries; Terraform is a trademark of HashiCorp, an IBM company. They are used only to identify the software this provider works with. You are responsible for using your hosts in compliance with the license agreement under which you obtained the hypervisor: see [Licensing and compliance](https://github.com/fdaforno/freecenter#licensing-and-compliance).
