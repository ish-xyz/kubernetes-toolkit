resource "virtualbox_vm" "node" {
  count     = 1
  name      = format("node-%02d", count.index + 1)
  image     = "https://app.vagrantup.com/alvistack/boxes/ubuntu-24.04/versions/20240726.0.0/providers/virtualbox/unknown/vagrant.box"
  cpus      = 2
  memory    = "1024 mib"
  user_data = ""

  network_adapter {
    type           = "nat"
  }
}

output "IPAddr" {
  value = element(virtualbox_vm.node.*.network_adapter.0.ipv4_address, 1)
}

# output "IPAddr_2" {
#   value = element(virtualbox_vm.node.*.network_adapter.0.ipv4_address, 2)
# }