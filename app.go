package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aquasecurity/bench-common/check"
	"github.com/aquasecurity/bench-common/util"
	"github.com/golang/glog"
	"github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
)

var benchmarkVersionMap = map[string]string{
	"cis-1.2": ">= 18.09",
	"cis-1.1": ">= 17.06, < 18.09",
	"cis-1.0": ">= 1.13.0, < 17.06",
}

func app(cmd *cobra.Command, args []string) {
	var version string
	var err error

	// Get version of Docker benchmark to run
	if dockerVersion != "" {
		version = dockerVersion
	} else {
		version, err = getDockerVersion()
		if err != nil {
			util.ExitWithError(
				fmt.Errorf("Version check failed: %s\nAlternatively, you can specify the version with --version",
					err))
		}
	}

	path, err := getDefinitionFilePath(version)
	if err != nil {
		util.ExitWithError(err)
	}

	controls, err := getControls(path)
	if err != nil {
		util.ExitWithError(err)
	}

	summary := runControls(controls, checkList)
	err = outputResults(controls, summary)
	if err != nil {
		util.ExitWithError(err)
	}
}

func outputResults(controls *check.Controls, summary check.Summary) error {
	// if we successfully ran some tests and it's json format, ignore the warnings
	if (summary.Fail > 0 || summary.Warn > 0 || summary.Pass > 0) && jsonFmt {
		out, err := controls.JSON()
		if err != nil {
			// util.ExitWithError(fmt.Errorf("failed to output in JSON format: %v", err))
			return err
		}
		util.PrintOutput(string(out), outputFile)
	} else {
		util.PrettyPrint(controls, summary, noRemediations, includeTestOutput)
	}

	return nil
}

func runControls(controls *check.Controls, checkList string) check.Summary {
	var summary check.Summary

	if checkList != "" {
		ids := util.CleanIDs(checkList)
		summary = controls.RunChecks(ids...)
	} else {
		summary = controls.RunGroup()
	}

	return summary
}

func getControls(path string) (*check.Controls, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	controls, err := check.NewControls([]byte(data), nil)
	if err != nil {
		return nil, err
	}

	return controls, err
}

// getDockerVersion returns the docker server engine version.
func getDockerVersion() (string, error) {
	cmd := exec.Command("docker", "version", "-f", "{{.Server.Version}}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return getDockerCisVersion(strings.TrimSpace(string(out)))
}

func getDefinitionFilePath(version string) (string, error) {
	filename := "definitions.yaml"

	glog.V(2).Info(fmt.Sprintf("Looking for config for  %s", version))

	path := filepath.Join(cfgDir, version)
	file := filepath.Join(path, filename)

	glog.V(2).Info(fmt.Sprintf("Looking for config file: %s\n", file))

	_, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	return file, nil
}

// getDockerCisVersion select the correct CIS version in compare to running docker version
// TBD ocp-3.9 auto detection
func getDockerCisVersion(stringVersion string) (string, error) {
	dockerVersion, err := trimVersion(stringVersion)

	if err != nil {
		return "", err
	}

	for benchVersion, dockerConstraints := range benchmarkVersionMap {
		currConstraints, err := version.NewConstraint(dockerConstraints)
		if err != nil {
			return "", err
		}
		if currConstraints.Check(dockerVersion) {
			glog.V(2).Info(fmt.Sprintf("docker version %s satisfies constraints %s", dockerVersion, currConstraints))
			return benchVersion, nil
		}
	}

	tooOldVersion, err := version.NewConstraint("< 1.13.0")
	if err != nil {
		return "", err
	}

	// Vesions before 1.13.0 are not supported by CIS.
	if tooOldVersion.Check(dockerVersion) {
		return "", fmt.Errorf("docker version %s is too old", stringVersion)
	}

	return "", fmt.Errorf("no suitable CIS version has been found for docker version %s", stringVersion)
}

// TrimVersion function remove all Matadate or  Prerelease parts
// because constraints.Check() can't handle comparison with it.
func trimVersion(stringVersion string) (*version.Version, error) {
	currVersion, err := version.NewVersion(stringVersion)
	if err != nil {
		return nil, err
	}

	if currVersion.Metadata() != "" || currVersion.Prerelease() != "" {
		tempStrVersion := strings.Trim(strings.Replace(fmt.Sprint(currVersion.Segments()), " ", ".", -1), "[]")
		currVersion, err = version.NewVersion(tempStrVersion)
		if err != nil {
			return nil, err
		}
	}

	return currVersion, nil
}
