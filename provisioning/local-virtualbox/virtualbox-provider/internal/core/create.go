package core

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	progressbar "github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

//TODO:
/*
	- add user data
		// user-data: ["{path: "/mnt/file.txt", dwata: "xyz"}"]
	- concurrency support
	- support multiple network settings

	LATER:
	- make unpacking idempotent
	- support vmdk files within vbox
	- support raw vmdk files
	- support ova
*/

func (core *CoreSettings) Create(vm *VirtualMachine) error {

	logrus.Infoln("fetching image", vm.Image)

	url, err := url.ParseRequestURI(vm.Image)
	if err != nil {
		return err
	}

	filename := filepath.Base(url.Path)
	fakeSHA := false
	downloadImage := true

	if vm.ImageSHA != "" {
		assumedPath := filepath.Join(core.LocalStore, fmt.Sprintf("%s-%s", vm.ImageSHA, filename))
		_, err := os.Open(assumedPath)
		if err == nil {
			downloadImage = false
		} else {
			// we assume the sha is fake anyway
			// we have no way to tell if it's legit or not
			fakeSHA = true
		}
	} else {
		// new temporary fake sha
		vm.ImageSHA = uuid.New().String()
		fakeSHA = true
	}

	imgPath := filepath.Join(core.LocalStore, fmt.Sprintf("%s-%s", vm.ImageSHA, filename))
	if downloadImage {
		if url.Scheme == "" {
			logrus.Infof("path is local, fetching from local disk at %s", url.Path)
			err = fetchFromLocal(url.Path, imgPath)
		} else {
			logrus.Infof("path is remote, fetching from resource at %s", url.String())
			err = fetchFromRemote(url, imgPath)
		}
		if err != nil {
			return err
		}
	}

	if fakeSHA {
		vm.ImageSHA, err = calculateSHA(imgPath)
		if err != nil {
			return fmt.Errorf("failed to calculate sha for downloaded image %s: %v", imgPath, err)
		}

		newPath := filepath.Join(core.LocalStore, fmt.Sprintf("%s-%s", vm.ImageSHA, filename))
		err = os.Rename(imgPath, newPath)
		if err != nil {
			return err
		}

		imgPath = newPath
	}

	logrus.Infof("image stored at: %s", imgPath)

	// create dest folder
	dstFolder := filepath.Join(core.LocalStore, vm.ImageSHA)
	err = os.MkdirAll(dstFolder, 0755)
	if err != nil {
		return fmt.Errorf("failed to create dstFolder for vbox conversion")
	}

	logrus.Infof("unpacking image in: %s", dstFolder)
	err = unpack(imgPath, dstFolder)
	if err != nil {
		return fmt.Errorf("failed to convert vbox image: %v", err)
	}

	if isSameImage(vm.Name, vm.ImageSHA) {

		logrus.Infof("vm with name %s already exists", vm.Name)
	} else {
		if getVMState(vm.Name) != VM_POWERED_OFF {
			runCommand(VBOX_MANAGE, "controlvm", vm.Name, "poweroff")
		}
		runCommand(VBOX_MANAGE, "unregistervm", vm.Name, "--delete-all")

		counter := 0
		for {
			time.Sleep(5 * time.Second)
			out, _ := runCommand(VBOX_MANAGE, "showvminfo", vm.Name)
			if out == "" || counter > 6 {
				break
			}
			counter++
		}

		if err := createWithOvf(vm, dstFolder); err != nil {
			// try with vmdk
			return fmt.Errorf("creation failed with: %v", err)
		}
		setGuestProperty(vm.Name, VM_IMAGE_SHA_KEY, vm.ImageSHA)
		logrus.Infoln("vm created successfully with ovf file!")
	}

	// Add necessary parameters
	if err := setVMMemory(vm.Name, vm.Memory); err != nil {
		return fmt.Errorf("error while running vboxmanage to modify memory: %v", err)
	}
	logrus.Infof("set %d MB of memory", vm.Memory)

	if err := setVMCPU(vm.Name, vm.CPU); err != nil {
		return fmt.Errorf("error while running vboxmanage to modify cpu settings: %v", err)
	}
	logrus.Infof("set %d cpus", vm.CPU)

	if err := setNetwork(vm.Name, vm.NetworkType); err != nil {
		return fmt.Errorf("error while running vboxmanage to modify network settings: %v", err)
	}
	logrus.Infof("modified network settings for vm %s", vm.Name)

	if getVMState(vm.Name) == VM_POWERED_OFF {
		_, err = runCommand(VBOX_MANAGE, "startvm", vm.Name, "--type", "headless")
		if err != nil {
			return fmt.Errorf("error while starting vm %s: %v", vm.Name, err)
		}
		logrus.Infoln("starting vm up...")
	} else {
		logrus.Infoln("machine already up and running.")
	}

	return nil
}

func isSameImage(vmName, desiredSHA string) bool {
	existingSHA, _ := getGuestProperty(vmName, VM_IMAGE_SHA_KEY)

	return existingSHA == desiredSHA
}

func setNetwork(vmName string, networkType string) error {
	out, _ := getGuestProperty(vmName, VM_NETWORK_KEY)
	if out == networkType {
		return nil
	}

	if getVMState(vmName) != VM_POWERED_OFF {
		runCommand(VBOX_MANAGE, "controlvm", vmName, "poweroff")
	}
	_, err := runCommand(VBOX_MANAGE, "modifyvm", vmName, "--nic1", networkType)
	if err != nil {
		return err
	}

	setGuestProperty(vmName, VM_NETWORK_KEY, networkType)
	return nil
}

func setVMMemory(vmName string, vmMemory int) error {

	memStr := fmt.Sprintf("%d", vmMemory)
	out, _ := getGuestProperty(vmName, VM_MEMORY_KEY)
	if out == memStr {
		return nil
	}

	if getVMState(vmName) != VM_POWERED_OFF {
		runCommand(VBOX_MANAGE, "controlvm", vmName, "poweroff")
	}
	_, err := runCommand(VBOX_MANAGE, "modifyvm", vmName, "--memory", memStr)
	if err != nil {
		return err
	}

	// set property for idempotence
	setGuestProperty(vmName, VM_MEMORY_KEY, memStr)

	return nil
}

func setVMCPU(vmName string, vmCPU int) error {

	cpuStr := fmt.Sprintf("%d", vmCPU)
	out, _ := getGuestProperty(vmName, VM_CPU_KEY)
	if out == cpuStr {
		return nil
	}

	if getVMState(vmName) != VM_POWERED_OFF {
		runCommand(VBOX_MANAGE, "controlvm", vmName, "poweroff")
	}
	_, err := runCommand(VBOX_MANAGE, "modifyvm", vmName, "--cpus", cpuStr)
	if err != nil {
		return err
	}

	// set property for idempotence
	setGuestProperty(vmName, VM_CPU_KEY, cpuStr)

	return nil
}

func createWithOvf(vm *VirtualMachine, dstFolder string) error {

	logrus.Infoln("looking for .ovf file in", dstFolder)
	ovfFiles, err := walkDir(dstFolder, []string{"ovf"})
	if err != nil {
		return fmt.Errorf("failed discovering ovf file: %v", err)
	}
	if len(ovfFiles) != 1 {
		return fmt.Errorf("unusual amount of .ovf file in store folder '%s', only 1 ovf file is supported", dstFolder)
	}

	logrus.Infof("registering new virtual machine with .ovf file %s", ovfFiles[0])
	output, err := runCommand(VBOX_MANAGE, "import", ovfFiles[0], "--vsys", "0", "--vmname", vm.Name)
	fmt.Println(output)
	if err != nil {
		return fmt.Errorf("failed to import .ovf file %s: %v", ovfFiles[0], err)
	}

	return nil
}

func fetchFromLocal(localFile, destFile string) error {

	// copy file
	in, err := os.Open(localFile)
	if err != nil {
		return fmt.Errorf("failed to open image file: %v", err)
	}
	defer in.Close()

	out, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("failed to open destination file for image: %v", err)
	}
	defer out.Close()

	_, err = io.Copy(in, out)
	if err != nil {
		return fmt.Errorf("failed while moving local file %s to destination %s: %v", localFile, destFile, err)
	}

	// sync and copy perms
	err = out.Sync()
	if err != nil {
		return fmt.Errorf("sync error: %v", err)
	}

	si, err := os.Stat(localFile)
	if err != nil {
		return fmt.Errorf("stat error: %v", err)
	}

	err = os.Chmod(destFile, si.Mode())
	if err != nil {
		return fmt.Errorf("chmod error: %v", err)
	}

	return nil
}

func fetchFromRemote(url *url.URL, destFile string) error {

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch remote resource: %s %v", url.String(), err)
	}
	defer resp.Body.Close()

	f, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("failed to open destination file for image: %v", err)
	}
	defer f.Close()

	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"downloading",
	)

	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	if err != nil {
		return fmt.Errorf("failed to download file: %s", url.String())
	}
	return nil
}

func unpack(src string, dst string) error {

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	buff := make([]byte, 512)
	if _, err = file.Read(buff); err != nil {
		return err
	}

	switch http.DetectContentType(buff) {
	case "application/x-gzip", "application/zip":
		return untargz(src, dst)
	default:
		if strings.HasPrefix(string(buff), "\x42\x5a\x68") {
			return untargz(src, dst)
		}
	}

	return untar(src, dst)
}

func createUserDataDisk(filename, userData string) error {
	// Create a 1MB file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write user data to the beginning of the file
	_, err = f.WriteString(userData)
	if err != nil {
		return err
	}

	// Pad the rest of the file with zeros to make it 1MB
	padding := make([]byte, 1024*1024-len(userData))
	_, err = f.Write(padding)
	return err
}

func attachUserDataDisk(vmName, diskPath string) error {
	_, err := runCommand(VBOX_MANAGE, "storageattach", vmName, "--storagectl", "SATA", "--port", "1", "--device", "0", "--type", "hdd", "--medium", diskPath)
	return err
}
