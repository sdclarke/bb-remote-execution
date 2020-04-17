package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buildbarn/bb-remote-execution/internal/mock"
	runner_pb "github.com/buildbarn/bb-remote-execution/pkg/proto/runner"
	"github.com/buildbarn/bb-remote-execution/pkg/runner"
	"github.com/buildbarn/bb-storage/pkg/filesystem"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExecutablePathResolvingRunner(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	defer ctrl.Finish()

	baseRunner := mock.NewMockRunner(ctrl)
	baseRunner.EXPECT().Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"/bin/bash", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	}).Return(&runner_pb.RunResponse{
		ExitCode: 0,
	}, nil)

	inputRootDirectory := mock.NewMockDirectoryCloser(ctrl)

	mockRootDirectoryOpener := func(inputRootPath string) (filesystem.DirectoryCloser, error) {
		return inputRootDirectory, nil
	}

	executablePathResolvingRunner := runner.NewExecutablePathResolvingRunner(baseRunner, mockRootDirectoryOpener)

	inputRootBin := mock.NewMockDirectoryCloser(ctrl)
	inputRootDirectory.EXPECT().Readlink("bin").Return("", errors.New(""))
	inputRootDirectory.EXPECT().EnterDirectory("bin").Return(inputRootBin, nil)
	inputRootDirectory.EXPECT().Close()

	inputRootBin.EXPECT().Lstat("bash").Return(filesystem.NewFileInfo("bash", filesystem.FileTypeExecutableFile), nil)
	inputRootBin.EXPECT().Close()

	runResponse, err := executablePathResolvingRunner.Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"bash", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	})
	require.NoError(t, err)

	require.Equal(t, runResponse, &runner_pb.RunResponse{
		ExitCode: 0,
	})
}

func TestExecutablePathResolvingRunnerSymlink(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	defer ctrl.Finish()

	baseRunner := mock.NewMockRunner(ctrl)
	baseRunner.EXPECT().Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"/bin/sh", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	}).Return(&runner_pb.RunResponse{
		ExitCode: 0,
	}, nil)

	inputRootDirectory := mock.NewMockDirectoryCloser(ctrl)

	mockRootDirectoryOpener := func(inputRootPath string) (filesystem.DirectoryCloser, error) {
		return inputRootDirectory, nil
	}

	executablePathResolvingRunner := runner.NewExecutablePathResolvingRunner(baseRunner, mockRootDirectoryOpener)

	inputRootBin := mock.NewMockDirectoryCloser(ctrl)
	inputRootDirectory.EXPECT().Readlink("bin").Return("", errors.New(""))
	inputRootDirectory.EXPECT().EnterDirectory("bin").Return(inputRootBin, nil)
	inputRootDirectory.EXPECT().Close()

	inputRootBin.EXPECT().Lstat("sh").Return(filesystem.NewFileInfo("sh", filesystem.FileTypeSymlink), nil)
	inputRootBin.EXPECT().Readlink("sh").Return("/bin/bash", nil)
	inputRootBin.EXPECT().Lstat("bash").Return(filesystem.NewFileInfo("bash", filesystem.FileTypeExecutableFile), nil)
	inputRootBin.EXPECT().Close()

	runResponse, err := executablePathResolvingRunner.Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"sh", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	})
	require.NoError(t, err)

	require.Equal(t, runResponse, &runner_pb.RunResponse{
		ExitCode: 0,
	})
}

func TestExecutablePathResolvingRunnerSymlinkLoop(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	defer ctrl.Finish()

	baseRunner := mock.NewMockRunner(ctrl)

	inputRootDirectory := mock.NewMockDirectoryCloser(ctrl)

	mockRootDirectoryOpener := func(inputRootPath string) (filesystem.DirectoryCloser, error) {
		return inputRootDirectory, nil
	}

	executablePathResolvingRunner := runner.NewExecutablePathResolvingRunner(baseRunner, mockRootDirectoryOpener)

	inputRootBin := mock.NewMockDirectoryCloser(ctrl)
	inputRootDirectory.EXPECT().Readlink("bin").Return("", errors.New(""))
	inputRootDirectory.EXPECT().EnterDirectory("bin").Return(inputRootBin, nil)
	inputRootDirectory.EXPECT().Close()

	inputRootBin.EXPECT().Lstat("sh").Return(filesystem.NewFileInfo("sh", filesystem.FileTypeSymlink), nil)
	inputRootBin.EXPECT().Readlink("sh").Return("/bin/bash", nil)
	inputRootBin.EXPECT().Lstat("bash").Return(filesystem.NewFileInfo("bash", filesystem.FileTypeSymlink), nil).Times(9)
	inputRootBin.EXPECT().Readlink("bash").Return("/bin/bash", nil).Times(9)
	inputRootBin.EXPECT().Close()

	runResponse, err := executablePathResolvingRunner.Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"sh", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	})
	require.Equal(t, status.Error(codes.NotFound, "Unable to find executable file: Executable file not found in PATH: The file is not executable"), err)
	require.Nil(t, runResponse)
}

func TestExecutablePathResolvingRunnerDirectorySymlink(t *testing.T) {
	ctrl, ctx := gomock.WithContext(context.Background(), t)
	defer ctrl.Finish()

	baseRunner := mock.NewMockRunner(ctrl)
	baseRunner.EXPECT().Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"/bin/sh", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	}).Return(&runner_pb.RunResponse{
		ExitCode: 0,
	}, nil)

	inputRootDirectory := mock.NewMockDirectoryCloser(ctrl)

	mockRootDirectoryOpener := func(inputRootPath string) (filesystem.DirectoryCloser, error) {
		return inputRootDirectory, nil
	}

	executablePathResolvingRunner := runner.NewExecutablePathResolvingRunner(baseRunner, mockRootDirectoryOpener)

	inputRootUsr := mock.NewMockDirectoryCloser(ctrl)
	inputRootUsrBin := mock.NewMockDirectoryCloser(ctrl)
	inputRootDirectory.EXPECT().Readlink("bin").Return("/usr/bin", nil)
	inputRootDirectory.EXPECT().Readlink("usr").Return("", errors.New(""))
	inputRootDirectory.EXPECT().EnterDirectory("usr").Return(inputRootUsr, nil)
	inputRootUsr.EXPECT().Readlink("bin").Return("", errors.New(""))
	inputRootUsr.EXPECT().EnterDirectory("bin").Return(inputRootUsrBin, nil)
	inputRootDirectory.EXPECT().Close()
	inputRootUsr.EXPECT().Close()

	inputRootUsrBin.EXPECT().Lstat("sh").Return(filesystem.NewFileInfo("sh", filesystem.FileTypeExecutableFile), nil)
	inputRootUsrBin.EXPECT().Close()

	runResponse, err := executablePathResolvingRunner.Run(ctx, &runner_pb.RunRequest{
		Arguments:            []string{"sh", "-c", "ls"},
		EnvironmentVariables: map[string]string{"PATH": "/bin"},
		WorkingDirectory:     "",
		StdoutPath:           "stdout",
		StderrPath:           "stderr",
		InputRootDirectory:   "root",
		TemporaryDirectory:   "tmp",
	})
	require.NoError(t, err)

	require.Equal(t, runResponse, &runner_pb.RunResponse{
		ExitCode: 0,
	})
}
