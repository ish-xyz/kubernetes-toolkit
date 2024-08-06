package core

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DEFAULT_HOME = ".virtualbox-provider/local-store"
)

type VirtualMachine struct {
	Name        string
	Image       string
	Memory      int
	CPU         int
	NetworkType string
	ImageSHA    string
}

type CoreSettings struct {
	LocalStore string
}

func NewVirtualMachine(name, img, imgsha string, mem, cpu int) *VirtualMachine {

	// TODO: make sure the image is vbox
	return &VirtualMachine{
		Name:        name,
		Image:       img,
		Memory:      mem,
		CPU:         cpu,
		NetworkType: "nat",
		ImageSHA:    imgsha,
	}
}

func NewCoreSettings(localStore string) (*CoreSettings, error) {

	// compute local store
	if localStore == "" {
		dirname, err := os.UserHomeDir()
		if err != nil || dirname == "" {
			return nil, fmt.Errorf("failed to detect home directory: %v", err)
		}
		localStore = filepath.Join(dirname, ".virtualbox-provider/local-store")
	}

	// ensure that local store exists
	if _, err := os.Stat(localStore); os.IsNotExist(err) {
		err := os.MkdirAll(localStore, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("failed to create work directory at %s: %v", localStore, err)
		}
	}

	return &CoreSettings{
		LocalStore: localStore,
	}, nil
}
