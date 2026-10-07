# Terraform examples

Examples for the [FreeCenter Terraform provider](../README.md). The files in this folder form one configuration; [`k8s-cluster`](k8s-cluster/) is a separate, self-contained one.

## Requirements

1. A running FreeCenter with your ESXi hosts registered. The examples look them up as `esxi-01` and `esxi-02`: change the names in `provider.tf`.
2. For the examples that deploy from a cloud image, the image on the datastore: download it from the New Virtual Machine wizard of FreeCenter ("Deploy from a cloud image"). It is saved as `cloud-images/<name>.vmdk`.
3. The provider. Until it is published on the Terraform Registry, build it and use a development override: see the [provider README](../README.md#building-and-using-a-local-build).

## Running

```bash
cd examples
cat > terraform.tfvars <<'VARS'
freecenter_address  = "http://freecenter.example.lan:8080"
freecenter_password = "..."
datastore           = "datastore1"
guest_password      = "..."
ssh_public_key      = "ssh-ed25519 AAAA... you@example"
auditor_password    = "..."
VARS

terraform plan
terraform apply
```

`terraform.tfvars` is ignored by git: keep your addresses and passwords there. To try a single example, use `-target`:

```bash
terraform apply -target=freecenter_vm.rocky
```

## What is here

| File | Shows |
|---|---|
| `provider.tf` | provider configuration, shared variables, host / datastore / network data sources |
| `vms-cloud-init.tf` | three Rocky Linux servers configured by cloud-init |
| `vm-ubuntu.tf` | an Ubuntu server with nginx |
| `vm-static-network.tf` | a static address through `network_config` |
| `vm-empty-disk.tf` | an empty virtual machine with two disks |
| `vm-iso-boot.tf` | booting from an ISO for a manual installation |
| `vm-template-clone.tf` | a template and clones of it, with a second network adapter |
| `vm-migrate.tf` | migrating a virtual machine to another host |
| `host-config.tf` | virtual switch, port group with VLAN, NTP, host services |
| `protection.tf` | snapshot, backup and restore, schedules, a read-only user |
| `k8s-cluster/` | a three-node Kubernetes cluster built with kubeadm |
