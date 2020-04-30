package runner

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/buildbarn/bb-remote-execution/pkg/proto/runner"
	"github.com/buildbarn/bb-storage/pkg/filesystem"
	"github.com/buildbarn/bb-storage/pkg/util"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RootDirectoryOpener is a type of function which takes the path of the
// input root directory of the action and returns a filesystem.Directory
// of the directory which will be the root when the action is run
type RootDirectoryOpener func(inputRootDirectory string) (filesystem.DirectoryCloser, error)

type executablePathResolvingRunner struct {
	base                Runner
	rootDirectoryOpener RootDirectoryOpener
}

// NewExecutablePathResolvingRunner modifies the arguments of the
// incoming RunRequest so that the first argument is an absolute path
// resolved using the path in the request, using the input root as the
// chroot if necessary
func NewExecutablePathResolvingRunner(base Runner, rootDirectoryOpener RootDirectoryOpener) Runner {
	return &executablePathResolvingRunner{
		base:                base,
		rootDirectoryOpener: rootDirectoryOpener,
	}
}

func (r *executablePathResolvingRunner) enterDir(file, chroot string) (filesystem.DirectoryCloser, error) {
	rootDir, err := r.rootDirectoryOpener(chroot)
	if err != nil {
		return nil, util.StatusWrap(err, "Failed to open root directory")
	}
	components := strings.FieldsFunc(file, func(r rune) bool { return r == '/' })
	dir := rootDir
	for n, component := range components[:len(components)-1] {
		if d, err := dir.Readlink(component); err == nil {
			return r.enterDir(filepath.Join(d, filepath.Join(components[n+1:]...)), chroot)
		}
		dir2, err := dir.EnterDirectory(component)
		if err != nil {
			return nil, util.StatusWrapf(err, "Failed to enter directory %#v", filepath.Join(components[:n+1]...))
		}
		dir.Close()
		dir = dir2
	}
	return dir, nil
}

func (r *executablePathResolvingRunner) findExecutable(file, chroot string) error {
	dir, err := r.enterDir(file, chroot)
	defer dir.Close()
	if err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		d, err := dir.Lstat(filepath.Base(file))
		if err != nil {
			return err
		}
		if m := d.Type(); m == filesystem.FileTypeExecutableFile {
			return nil
		} else if m != filesystem.FileTypeSymlink {
			return status.Error(codes.NotFound, "The file is not executable")
		}
		link, err := dir.Readlink(filepath.Base(file))
		if err != nil {
			return err
		}
		if strings.ContainsRune(link, '/') && filepath.Dir(link) != filepath.Dir(file) {
			dir, err = r.enterDir(link, chroot)
			if err != nil {
				return err
			}
		}
		file = link
	}
	return status.Error(codes.NotFound, "The file is not executable")
}

func (r *executablePathResolvingRunner) lookPath(request *runner.RunRequest, path string) (string, error) {
	file := request.Arguments[0]
	if strings.ContainsRune(file, '/') {
		if err := r.findExecutable(file, request.InputRootDirectory); err != nil {
			return file, util.StatusWrap(err, "Executable file not found in PATH")
		}
		return file, nil
	}
	var err error
	for _, dir := range filepath.SplitList(path) {
		path := filepath.Join(dir, file)
		if err = r.findExecutable(path, request.InputRootDirectory); err == nil {
			return path, nil
		}
	}
	return file, util.StatusWrap(err, "Executable file not found in PATH")
}

func (r *executablePathResolvingRunner) Run(ctx context.Context, request *runner.RunRequest) (*runner.RunResponse, error) {
	if len(request.Arguments) < 1 {
		return nil, status.Error(codes.InvalidArgument, "Insufficient number of command arguments")
	}
	path, present := request.EnvironmentVariables["PATH"]
	if !present && !strings.ContainsRune(request.Arguments[0], '/') {
		return nil, status.Error(codes.InvalidArgument, "No PATH in command's environment variables")
	}
	fullPath, err := r.lookPath(request, path)
	if err != nil {
		return nil, util.StatusWrap(err, "Unable to find executable file")
	}
	request.Arguments[0] = filepath.Join("/", filepath.Base(filepath.Dir(fullPath)), request.Arguments[0])
	return r.base.Run(ctx, request)
}
