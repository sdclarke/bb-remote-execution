// +build linux

package filesystem

import (
	"os"
	"syscall"

	"github.com/buildbarn/bb-storage/pkg/filesystem"
	"github.com/buildbarn/bb-storage/pkg/util"
)

// Device stores the name, device number and mode of a device to be used
// when creating device nodes in the input root dev directory
type Device struct {
	Name string
	Rdev uint64
}

// CreateDev will take a directory and a list of devices and make nodes
// of those devices in the /dev subdirectory of the given directory
func CreateDev(inputRootDirectory filesystem.Directory, devices []Device) error {
	if err := inputRootDirectory.Mkdir("dev", 0777); err != nil && !os.IsExist(err) {
		return util.StatusWrap(err, "Unable to create dev directory in input root")
	}
	devDir, err := inputRootDirectory.EnterDirectory("dev")
	if err != nil {
		return util.StatusWrap(err, "Unable to enter dev directory in input root")
	}
	for _, device := range devices {
		if _, err := devDir.Lstat(device.Name); err == nil {
			continue
		}
		if err := devDir.Mknod(device.Name, os.FileMode(syscall.S_IFCHR|0666), int(device.Rdev)); err != nil {
			return util.StatusWrapf(err, "Mknod failed for device %#v", device.Name)
		}
	}
	return nil
}
