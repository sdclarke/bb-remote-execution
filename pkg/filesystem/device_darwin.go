// +build darwin

package filesystem

import (
	"os"

	"github.com/buildbarn/bb-storage/pkg/filesystem"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Device stores the name, device number and mode of a device to be used
// when creating device nodes in the input root dev directory
type Device struct {
	Name string
	Rdev int32
}

// CreateDev will return a fixed error on darwin
func CreateDev(inputRootDirectory filesystem.Directory, devices []Device) error {
	return status.Error(codes.Unimplemented, "Creation of device nodes is not supported on darwin")
}
