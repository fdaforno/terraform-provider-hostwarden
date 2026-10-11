output "vm_ids" {
  value       = { for k, v in hostwarden_vm.node : k => v.id }
  description = "Hostwarden id (<host_id>/<vm_id>) of each node."
}

output "ip_addresses" {
  value       = { for k, v in hostwarden_vm.node : k => v.ip_address }
  description = "Address of each node, as reported by its guest tools."
}

output "kubectl_hint" {
  value       = "ssh root@${local.api_ip} kubectl get nodes"
  description = "Command that shows the state of the cluster."
}
