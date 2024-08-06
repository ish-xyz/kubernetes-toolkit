package core

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	VBOX_MANAGE      = "VBoxManage"
	VM_CPU_KEY       = "vbp_cpu"
	VM_MEMORY_KEY    = "vbp_memory"
	VM_IMAGE_SHA_KEY = "vbp_imageSHA"
	VM_NETWORK_KEY   = "vbp_networkType"
	VM_POWERED_OFF   = "poweredoff"
)

func runCommand(prog string, args ...string) (string, error) {

	cmd := exec.Command(prog, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func walkDir(root string, exts []string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		for _, s := range exts {
			if strings.HasSuffix(path, "."+s) {
				files = append(files, path)
				return nil
			}
		}

		return nil
	})
	return files, err
}

func untar(src string, dst string) error {

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}

func untargz(src string, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}

func calculateSHA(resource string) (string, error) {

	f, err := os.Open(resource)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	bs := h.Sum(nil)

	return fmt.Sprintf("%x", bs), nil
}

func getGuestProperty(vmName, propertyName string) (string, error) {
	out, err := runCommand(VBOX_MANAGE, "guestproperty", "get", vmName, propertyName)
	if err != nil {
		return "", err
	}

	if strings.Contains(out, "No value set!") {
		return "", fmt.Errorf("value not set for property %s", propertyName)
	}

	data := strings.Split(out, ":")

	returnValue := cleanString(data[1])

	return returnValue, nil
}

func setGuestProperty(vmName, propertyName, value string) error {

	value = cleanString(value)
	_, err := runCommand(VBOX_MANAGE, "guestproperty", "set", vmName, propertyName, value)
	if err != nil {
		return err
	}

	return nil
}

func getVMState(vmName string) string {
	var state string
	out, err := runCommand(VBOX_MANAGE, "showvminfo", vmName)
	if err != nil {
		return ""
	}
	out = strings.Replace(out, "\r", "", -1)
	data := strings.Split(out, "\n")

	for x := range data {
		if strings.Contains(data[x], "State:") {
			state = strings.Split(data[x], ":")[1]
			state = strings.Replace(strings.Split(state, "(")[0], " ", "", -1)
			state = cleanString(state)
		}
	}
	return state
}

func cleanString(value string) string {

	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimPrefix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	value = strings.TrimPrefix(value, "\r")
	value = strings.TrimSuffix(value, " ")
	value = strings.TrimPrefix(value, " ")

	return value
}
