// Copyright 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vmware-tanzu/tanzu-framework/hack/packages/package-tools/constants"
	"github.com/vmware-tanzu/tanzu-framework/hack/packages/package-tools/utils"
)

// packageVendirSyncCmd is for sync the package
var packageVendirSyncCmd = &cobra.Command{
	Use:   "vendir sync",
	Short: "Package vendir sync",
	RunE:  runPackageVendirSync,
}

func init() {
	rootCmd.AddCommand(packageVendirSyncCmd)
	packageVendirSyncCmd.Flags().StringVar(&packageRepository, "repository", "", "Package repository of the packages to be synced")
}

func runPackageVendirSync(cmd *cobra.Command, args []string) error {
	projectRootDir, err := utils.GetProjectRootDir()
	if err != nil {
		return err
	}

	repositoryPath := filepath.Join(projectRootDir, "packages", packageRepository)
	toolsBinDir := filepath.Join(projectRootDir, constants.ToolsBinDirPath)
	files, err := os.ReadDir(repositoryPath)
	if err != nil {
		return fmt.Errorf("couldn't read repository directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("Syncing package %s\n", file.Name())
			packagePath := filepath.Join(repositoryPath, file.Name())
			if err := syncPackage(packagePath, toolsBinDir); err != nil {
				return err
			}
		}
	}
	return nil
}

func syncPackage(packagePath, toolsBinDir string) error {
	err := os.Chdir(packagePath)
	if err != nil {
		return fmt.Errorf("couldn't change to directory %s: %w", packagePath, err)
	}

	cmd := exec.Command(filepath.Join(toolsBinDir, "vendir"), "sync") // #nosec G204
	var errBytes bytes.Buffer
	cmd.Stderr = &errBytes
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("couldn't vendir sync package: %s", errBytes.String())
	}
	return nil
}
