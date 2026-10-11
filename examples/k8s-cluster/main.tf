# A three-node Kubernetes cluster (one control-plane, two workers) on an ESXi
# host managed by Hostwarden.
#
# The nodes are deployed from the Rocky Linux 9 cloud image with static
# addresses and provisioned by cloud-init: containerd, kubelet / kubeadm /
# kubectl, then kubeadm init on the control-plane and kubeadm join (with
# retries) on the workers. The CNI is Flannel.
#
# Usage: see README.md.

terraform {
  required_version = ">= 1.5"

  required_providers {
    hostwarden = {
      source = "registry.terraform.io/fdaforno/hostwarden"
    }
  }
}

provider "hostwarden" {
  address = var.hostwarden_address
  # Credentials from the environment: HOSTWARDEN_TOKEN, or HOSTWARDEN_USERNAME and HOSTWARDEN_PASSWORD.
}

data "hostwarden_host" "main" {
  name = var.host_name
}

locals {
  nodes  = var.nodes
  api_ip = one([for n in var.nodes : n.ip if n.role == "control-plane"])
}

resource "hostwarden_vm" "node" {
  for_each = local.nodes

  host_id  = data.hostwarden_host.main.id
  name     = each.key
  guest_id = "rockylinux_64Guest"

  datastore = var.datastore
  num_cpus  = each.value.role == "control-plane" ? var.control_plane_cpus : var.worker_cpus
  memory_mb = each.value.role == "control-plane" ? var.control_plane_memory_mb : var.worker_memory_mb
  disk_sizes_gb = [
    each.value.role == "control-plane" ? var.control_plane_disk_gb : var.worker_disk_gb
  ]
  disk_thin = true

  network      = var.network
  adapter_type = "vmxnet3"

  cloud_image = var.cloud_image

  cloud_init {
    # Static networking via user_data write_files + runcmd (nmcli), NOT the
    # network_config field — cloud-init 24.x on Rocky 9 misparses yamlencode()
    # output (block sequences at same indent as keys) and crashes.
    user_data = templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
      hostname      = each.key
      ip            = each.value.ip
      role          = each.value.role
      prefix        = var.prefix
      gateway       = var.gateway
      dns_servers   = var.dns_servers
      api_ip        = local.api_ip
      pod_cidr      = var.pod_cidr
      k8s_version   = var.k8s_version
      join_token    = var.join_token
      root_password = var.root_password
    })
  }

  power = "on"
}
