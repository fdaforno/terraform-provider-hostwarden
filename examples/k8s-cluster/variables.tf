variable "hostwarden_address" {
  description = "URL of the Hostwarden server."
  type        = string
  default     = "http://localhost:8080"
}

variable "host_name" {
  description = "Name of the ESXi host (as registered in Hostwarden) the nodes run on."
  type        = string
  default     = "esxi-01"
}

variable "nodes" {
  description = "Nodes of the cluster by name: static address and role. Exactly one must have the role control-plane."
  type = map(object({
    ip   = string
    role = string
  }))
  default = {
    k8s-cp-1 = { ip = "192.168.1.71", role = "control-plane" }
    k8s-w-1  = { ip = "192.168.1.72", role = "worker" }
    k8s-w-2  = { ip = "192.168.1.73", role = "worker" }
  }

  validation {
    condition     = length([for n in var.nodes : n if n.role == "control-plane"]) == 1 && alltrue([for n in var.nodes : contains(["control-plane", "worker"], n.role)])
    error_message = "Each node has the role control-plane or worker, and exactly one is the control-plane."
  }
}

variable "root_password" {
  description = "Root password of the nodes (SSH and console)."
  type        = string
  sensitive   = true
}

variable "join_token" {
  description = "Pre-shared kubeadm token ([a-z0-9]{6}.[a-z0-9]{16}): used by init on the control-plane and by join on the workers."
  type        = string
  sensitive   = true
}

variable "datastore" {
  description = "Datastore of the nodes. It must hold the cloud image."
  type        = string
  default     = "datastore1"
}

variable "network" {
  description = "Port group of the network adapter of the nodes."
  type        = string
  default     = "VM Network"
}

variable "cloud_image" {
  description = "Rocky Linux cloud image on the datastore (relative path)."
  type        = string
  default     = "cloud-images/rocky-9.vmdk"
}

variable "prefix" {
  description = "Network prefix length of the node addresses."
  type        = number
  default     = 24
}

variable "gateway" {
  description = "Default gateway of the nodes."
  type        = string
  default     = "192.168.1.1"
}

variable "dns_servers" {
  description = "Two name servers for the nodes."
  type        = list(string)
  default     = ["192.168.1.1", "9.9.9.9"]

  validation {
    condition     = length(var.dns_servers) == 2
    error_message = "Give exactly two name servers."
  }
}

variable "k8s_version" {
  description = "Kubernetes minor version to install (e.g. 1.33): package repository, packages and kubeadm images."
  type        = string
  default     = "1.33"
}

variable "pod_cidr" {
  description = "Pod network CIDR (Flannel). It must not overlap the network of the nodes."
  type        = string
  default     = "10.244.0.0/16"
}

variable "control_plane_cpus" {
  type    = number
  default = 2
}

variable "control_plane_memory_mb" {
  type    = number
  default = 4096
}

variable "control_plane_disk_gb" {
  type    = number
  default = 40
}

variable "worker_cpus" {
  type    = number
  default = 4
}

variable "worker_memory_mb" {
  type    = number
  default = 8192
}

variable "worker_disk_gb" {
  type    = number
  default = 60
}
