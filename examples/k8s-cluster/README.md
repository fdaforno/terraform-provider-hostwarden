# A three-node Kubernetes cluster with kubeadm

One control-plane and two workers on an ESXi host managed by FreeCenter. The nodes are deployed from the Rocky Linux 9 cloud image with static addresses and provisioned entirely by cloud-init:

- containerd with the systemd cgroup driver;
- kubelet, kubeadm and kubectl of `var.k8s_version`;
- `kubeadm init` on the control-plane and `kubeadm join` (retried until the API server answers) on the workers;
- Flannel as the CNI (`var.pod_cidr`).

**This is a lab setup, not a hardened one.** SELinux is permissive and firewalld is disabled on the nodes, root can sign in over SSH with a password, and the workers join with a pre-shared token and `--discovery-token-unsafe-skip-ca-verification`. Choose your own password and token, and keep them out of version control.

## Requirements

- A host registered in FreeCenter, with the Rocky 9 cloud image on its datastore as `cloud-images/rocky-9.vmdk` (download it from the New Virtual Machine wizard of FreeCenter).
- Three free addresses on the network of the nodes.
- The provider: see the [provider README](../../README.md#building-and-using-a-local-build) for using a local build.

## Apply

```bash
cd examples/k8s-cluster
cp terraform.tfvars.example terraform.tfvars    # then edit: addresses, password, token
export FREECENTER_USERNAME=admin FREECENTER_PASSWORD=...   # or FREECENTER_TOKEN
terraform init
terraform apply
```

The cloud-init provisioning takes 5 to 10 minutes (packages and container images). Then:

```bash
ssh root@<control-plane address> kubectl get nodes     # three nodes, Ready
```

Changing `k8s_version`, the sizing or the addresses means recreating the nodes: `terraform destroy`, then `apply` again. cloud-init only runs on the first boot.
