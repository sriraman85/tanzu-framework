// Copyright 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/vmware-tanzu/tanzu-framework/pkg/v1/cli/carvelhelpers"
	"github.com/vmware-tanzu/tanzu-framework/pkg/v1/tkg/log"
	"github.com/vmware-tanzu/tanzu-framework/pkg/v1/tkg/managementcomponents"
	"github.com/vmware-tanzu/tanzu-framework/pkg/v1/tkg/tkgconfigreaderwriter"
	"github.com/vmware-tanzu/tanzu-framework/pkg/v1/tkg/utils"
)

// InstallManagementComponents install management components to the cluster
func (c *TkgClient) InstallManagementComponents(kubeconfig, kubecontext string) error {
	managementPackageRepoImage, err := c.tkgBomClient.GetManagementPackageRepositoryImage()
	if err != nil {
		return errors.Wrap(err, "unable to get management package repository image")
	}

	// Override management package repository image if specified as part of below environment variable
	// NOTE: this override is only for testing purpose and we don't expect this to be used in production scenario
	mprImage := os.Getenv("MANAGEMENT_PACKAGE_REPO_IMAGE")
	if mprImage != "" {
		managementPackageRepoImage = mprImage
	}

	// Get TKG package's values file
	tkgPackageValuesFile, err := c.getTKGPackageConfigValuesFile()
	if err != nil {
		return err
	}

	// Get kapp-controller configuration file
	kappControllerConfigFile, err := c.getKappControllerConfigFile()
	if err != nil {
		return err
	}

	managementcomponentsInstallOptions := managementcomponents.ManagementComponentsInstallOptions{
		ClusterOptions: managementcomponents.ClusterOptions{
			Kubeconfig:  kubeconfig,
			Kubecontext: kubecontext,
		},
		KappControllerOptions: managementcomponents.KappControllerOptions{
			KappControllerConfigFile:       kappControllerConfigFile,
			KappControllerInstallNamespace: "tkg-system",
		},
		ManagementPackageRepositoryOptions: managementcomponents.ManagementPackageRepositoryOptions{
			ManagementPackageRepoImage: managementPackageRepoImage,
			TKGPackageValuesFile:       tkgPackageValuesFile,
		},
	}

	err = managementcomponents.InstallManagementComponents(&managementcomponentsInstallOptions)

	// Remove intermediate config files if err is empty
	if err == nil {
		os.Remove(tkgPackageValuesFile)
		os.Remove(kappControllerConfigFile)
	}

	return nil
}

func (c *TkgClient) getTKGPackageConfigValuesFile() (string, error) {
	userProviderConfigValues, err := c.getUserConfigVariableValueMap()
	if err != nil {
		return "", err
	}

	valuesFile, err := managementcomponents.GetTKGPackageConfigValuesFileFromUserConfig(userProviderConfigValues)
	if err != nil {
		return "", err
	}

	return valuesFile, nil
}

func (c *TkgClient) getUserConfigVariableValueMap() (map[string]string, error) {
	path, err := c.tkgConfigPathsClient.GetConfigDefaultsFilePath()
	if err != nil {
		return nil, err
	}

	return c.GetUserConfigVariableValueMap(path, c.TKGConfigReaderWriter())
}

func (c *TkgClient) getUserConfigVariableValueMapFile() (string, error) {
	userConfigValues, err := c.getUserConfigVariableValueMap()
	if err != nil {
		return "", err
	}

	configBytes, err := yaml.Marshal(userConfigValues)
	if err != nil {
		return "", err
	}

	prefix := []byte(`#@data/values
#@overlay/match-child-defaults missing_ok=True
---
`)
	configBytes = append(prefix, configBytes...)

	configFile, err := utils.CreateTempFile("", "*.yaml")
	if err != nil {
		return "", err
	}
	err = utils.WriteToFile(configFile, configBytes)
	if err != nil {
		return "", err
	}
	return configFile, nil
}

func (c *TkgClient) getKappControllerConfigFile() (string, error) {
	kappControllerPackageImage, err := c.tkgBomClient.GetKappControllerPackageImage()
	if err != nil {
		return "", err
	}

	path, err := c.tkgConfigPathsClient.GetTKGProvidersDirectory()
	if err != nil {
		return "", err
	}
	kappControllerValuesDirPath := filepath.Join(path, "kapp-controller-values")

	userConfigValuesFile, err := c.getUserConfigVariableValueMapFile()
	if err != nil {
		return "", err
	}

	defer func() {
		// Remove intermediate config files if err is empty
		if err == nil {
			os.Remove(userConfigValuesFile)
		}
	}()

	log.V(6).Infof("User ConfigValues File: %v", userConfigValuesFile)

	kappControllerConfigFile, err := ProcessKappControllerPackage(kappControllerPackageImage, userConfigValuesFile, kappControllerValuesDirPath)
	if err != nil {
		return "", err
	}

	return kappControllerConfigFile, nil
}

func ProcessKappControllerPackage(kappControllerPackageImage, userConfigValuesFile, kappControllerValuesDirPath string) (string, error) {
	kappControllerValuesFile, err := GetKappControllerConfigValuesFile(userConfigValuesFile, kappControllerValuesDirPath)
	if err != nil {
		return "", err
	}

	defer func() {
		// Remove intermediate config files if err is empty
		if err == nil {
			os.Remove(kappControllerValuesFile)
		}
	}()

	log.V(6).Infof("Kapp-controller values-file: %v", kappControllerValuesFile)

	configBytes, err := carvelhelpers.ProcessCarvelPackage(kappControllerPackageImage, kappControllerValuesFile)
	if err != nil {
		return "", err
	}

	configFile, err := utils.CreateTempFile("", "")
	if err != nil {
		return "", err
	}
	err = utils.WriteToFile(configFile, configBytes)
	if err != nil {
		return "", err
	}

	log.V(6).Infof("Kapp-controller configuration file: %v", configFile)
	return configFile, nil
}

func GetKappControllerConfigValuesFile(userConfigValuesFile, kappControllerValuesDir string) (string, error) {
	kappControllerValuesBytes, err := carvelhelpers.ProcessYTTPackage(kappControllerValuesDir, userConfigValuesFile)
	if err != nil {
		return "", err
	}

	prefix := []byte(`#@data/values
#@overlay/match-child-defaults missing_ok=True
#@overlay/replace
---
`)
	kappControllerValuesBytes = append(prefix, kappControllerValuesBytes...)
	kappControllerValuesFile, err := utils.CreateTempFile("", "*.yaml")
	if err != nil {
		return "", err
	}
	err = utils.WriteToFile(kappControllerValuesFile, kappControllerValuesBytes)
	if err != nil {
		return "", err
	}

	return kappControllerValuesFile, nil
}

// GetUserConfigVariableValueMap is a specific implementation expecting to use a flat key-value
// file to provide a source of keys to filter for the valid user provided values.
// For example, this function uses config_default.yaml filepath to find relevant config variables
// and returns the config map of user provided variable among all applicable config variables
func (c *TkgClient) GetUserConfigVariableValueMap(configDefaultFilePath string, rw tkgconfigreaderwriter.TKGConfigReaderWriter) (map[string]string, error) {
	bytes, err := os.ReadFile(configDefaultFilePath)
	if err != nil {
		return nil, err
	}

	variables, err := GetConfigVariableListFromYamlData(bytes)
	if err != nil {
		return nil, err
	}

	userProvidedConfigValues := map[string]string{}
	for _, k := range variables {
		if v, e := rw.Get(k); e == nil {
			userProvidedConfigValues[k] = v
		}
	}

	return userProvidedConfigValues, nil
}

func GetConfigVariableListFromYamlData(bytes []byte) ([]string, error) {
	configValues := map[string]interface{}{}
	err := yaml.Unmarshal(bytes, &configValues)
	if err != nil {
		return nil, errors.Wrap(err, "error while unmarshaling")
	}

	keys := make([]string, 0, len(configValues))
	for k := range configValues {
		keys = append(keys, k)
	}

	return keys, nil
}
