package cmd

import (
	"virtualbox-provider/internal/core"

	"github.com/spf13/cobra"
)

var (
	rootCmd = cobra.Command{
		RunE: start,
	}
)

func Execute() {
	rootCmd.Execute()
}

func start(cmd *cobra.Command, args []string) error {
	// other image => //""https://app.vagrantup.com/alvistack/boxes/ubuntu-24.04/versions/20240726.0.0/providers/virtualbox/unknown/vagrant.box"
	vm := core.NewVirtualMachine(
		"virtualbox-provider",
		"https://app.vagrantup.com/bento/boxes/ubuntu-20.04/versions/202401.31.0/providers/virtualbox/amd64/vagrant.box",
		"bd2930202ae51ec8476d0c2956b8d971fb7414fe1a29f6a469c5cff0769d7435",
		1024,
		2,
	)
	cs, _ := core.NewCoreSettings("/home/waffle34/.virtualbox-provider/local-store")

	err := cs.Create(vm)

	if err != nil {
		return err
	}

	return nil
}

/*
	resource "virtual_machine" "blah" {
		image = ""
		cpu =
		...
	}

*/
