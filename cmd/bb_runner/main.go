package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildbarn/bb-remote-execution/pkg/proto/configuration/bb_runner"
	runner_pb "github.com/buildbarn/bb-remote-execution/pkg/proto/runner"
	"github.com/buildbarn/bb-remote-execution/pkg/proto/tmp_installer"
	"github.com/buildbarn/bb-remote-execution/pkg/runner"
	"github.com/buildbarn/bb-storage/pkg/filesystem"
	"github.com/buildbarn/bb-storage/pkg/global"
	bb_grpc "github.com/buildbarn/bb-storage/pkg/grpc"
	"github.com/buildbarn/bb-storage/pkg/util"
	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: bb_runner bb_runner.jsonnet")
	}
	var configuration bb_runner.ApplicationConfiguration
	if err := util.UnmarshalConfigurationFromFile(os.Args[1], &configuration); err != nil {
		log.Fatalf("Failed to read configuration from %s: %s", os.Args[1], err)
	}
	if err := global.ApplyConfiguration(configuration.Global); err != nil {
		log.Fatal("Failed to apply global configuration options: ", err)
	}

	buildDirectory, err := filesystem.NewLocalDirectory(configuration.BuildDirectoryPath)
	if err != nil {
		log.Fatal("Failed to open build directory: ", err)
	}

	rootDirectoryOpener := func(inputRootDirectory string) (filesystem.DirectoryCloser, error) {
		var dir filesystem.DirectoryCloser
		if configuration.ChrootIntoInputRoot {
			components := strings.FieldsFunc(inputRootDirectory, func(r rune) bool { return r == '/' })
			dir = buildDirectory
			for n, component := range components {
				dir2, err := dir.EnterDirectory(component)
				if err != nil {
					return nil, util.StatusWrapf(err, "Failed to enter directory %#v", filepath.Join(components[:n+1]...))
				}
				if dir != buildDirectory {
					dir.Close()
				}
				dir = dir2
			}
		} else {
			binDir, err := filesystem.NewLocalDirectory("/")
			if err != nil {
				return nil, err
			}
			dir = binDir
		}
		return dir, nil
	}

	r := runner.NewExecutablePathResolvingRunner(
		runner.NewLocalRunner(
			buildDirectory,
			configuration.BuildDirectoryPath,
			configuration.SetTmpdirEnvironmentVariable,
			configuration.ChrootIntoInputRoot),
		rootDirectoryOpener)

	// When temporary directories need cleaning prior to executing a build
	// action, attach a series of TemporaryDirectoryCleaningRunners.
	for _, d := range configuration.TemporaryDirectories {
		directory, err := filesystem.NewLocalDirectory(d)
		if err != nil {
			log.Fatalf("Failed to open temporary directory %#v: %s", d, err)
		}
		r = runner.NewTemporaryDirectoryCleaningRunner(r, directory, d)
	}

	// Calling into a helper process to set up access to temporary
	// directories prior to the execution of build actions.
	if configuration.TemporaryDirectoryInstaller != nil {
		tmpInstallerConnection, err := bb_grpc.NewGRPCClientFromConfiguration(configuration.TemporaryDirectoryInstaller)
		if err != nil {
			log.Fatal("Failed to create temporary directory installer RPC client: ", err)
		}
		tmpInstaller := tmp_installer.NewTemporaryDirectoryInstallerClient(tmpInstallerConnection)
		for {
			_, err := tmpInstaller.CheckReadiness(context.Background(), &empty.Empty{})
			if err == nil {
				break
			}
			log.Print("Temporary directory installer is not ready yet: ", err)
			time.Sleep(3 * time.Second)
		}
		r = runner.NewTemporaryDirectoryInstallingRunner(r, tmpInstaller)
	}

	log.Fatal(
		"gRPC server failure: ",
		bb_grpc.NewGRPCServersFromConfigurationAndServe(
			configuration.GrpcServers,
			func(s *grpc.Server) {
				runner_pb.RegisterRunnerServer(s, runner.NewRunnerServer(r))
			}))
}
