package apps

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega/gbytes"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega/gexec"
	archive_helpers "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/pivotal-golang/archiver/extractor/test_helper"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
)

var _ = Describe("Admin Buildpacks", func() {
	var (
		appName       string
		BuildpackName string

		appPath string

		buildpackPath        string
		buildpackArchivePath string
	)

	matchingFilename := func(appName string) string {
		return fmt.Sprintf("simple-buildpack-please-match-%s", appName)
	}

	AfterEach(func() {
		app_helpers.AppReport(appName, DEFAULT_TIMEOUT)
	})

	type appConfig struct {
		Empty bool
	}

	setupBuildpack := func(appConfig appConfig) {
		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			BuildpackName = RandomName()
			appName = PrefixedRandomName("CATS-APP-")

			tmpdir, err := ioutil.TempDir(os.TempDir(), "matching-app")
			Expect(err).ToNot(HaveOccurred())

			appPath = tmpdir

			tmpdir, err = ioutil.TempDir(os.TempDir(), "matching-buildpack")
			Expect(err).ToNot(HaveOccurred())

			buildpackPath = tmpdir
			buildpackArchivePath = path.Join(buildpackPath, "buildpack.zip")

			archive_helpers.CreateZipArchive(buildpackArchivePath, []archive_helpers.ArchiveFile{
				{
					Name: "bin/compile",
					Body: `#!/usr/bin/env bash

sleep 5 # give loggregator time to start streaming the logs

echo "Staging with Simple Buildpack"

sleep 10
`,
				},
				{
					Name: "bin/detect",
					Body: fmt.Sprintf(`#!/bin/bash

if [ -f "${1}/%s" ]; then
  echo Simple
else
  echo no
  exit 1
fi
`, matchingFilename(appName)),
				},
				{
					Name: "bin/release",
					Body: `#!/usr/bin/env bash

cat <<EOF
---
config_vars:
  PATH: bin:/usr/local/bin:/usr/bin:/bin
  FROM_BUILD_PACK: "yes"
default_process_types:
  web: while true; do { echo -e 'HTTP/1.1 200 OK\r\n'; echo "hi from a simple admin buildpack"; } | nc -l \$PORT; done
EOF
`,
				},
			})

			if !appConfig.Empty {
				_, err = os.Create(path.Join(appPath, matchingFilename(appName)))
				Expect(err).ToNot(HaveOccurred())
			}

			_, err = os.Create(path.Join(appPath, "some-file"))
			Expect(err).ToNot(HaveOccurred())

			createBuildpack := cf.Cf("create-buildpack", BuildpackName, buildpackArchivePath, "0").Wait(DEFAULT_TIMEOUT)
			Expect(createBuildpack).Should(Exit(0))
			Expect(createBuildpack).Should(Say("Creating"))
			Expect(createBuildpack).Should(Say("OK"))
			Expect(createBuildpack).Should(Say("Uploading"))
			Expect(createBuildpack).Should(Say("OK"))
		})
	}

	deleteBuildpack := func() {
		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			Expect(cf.Cf("delete-buildpack", BuildpackName, "-f").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		})
	}

	deleteApp := func() {
		command := cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)
		Expect(command).To(Exit(0))
		Expect(command).To(Say(fmt.Sprintf("Deleting app %s", appName)))
	}

	itIsUsedForTheApp := func() {
		Expect(cf.Cf("push", appName, "--no-start", "-m", DEFAULT_MEMORY_LIMIT, "-p", appPath, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		app_helpers.SetBackend(appName)

		start := cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)
		Expect(start).To(Exit(0))
		Expect(start).To(Say("Staging with Simple Buildpack"))
	}

	itDoesNotDetectForEmptyApp := func() {
		Expect(cf.Cf("push", appName, "--no-start", "-m", DEFAULT_MEMORY_LIMIT, "-p", appPath, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		app_helpers.SetBackend(appName)

		start := cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)
		Expect(start).To(Exit(1))
		Expect(start).To(Say("NoAppDetectedError"))
	}

	itDoesNotDetectWhenBuildpackDisabled := func() {
		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			var response cf.QueryResponse

			cf.ApiRequest("GET", "/v2/buildpacks?q=name:"+BuildpackName, &response, DEFAULT_TIMEOUT)

			Expect(response.Resources).To(HaveLen(1))

			buildpackGuid := response.Resources[0].Metadata.Guid

			cf.ApiRequest(
				"PUT",
				"/v2/buildpacks/"+buildpackGuid,
				nil,
				DEFAULT_TIMEOUT,
				`{"enabled":false}`,
			)
		})

		Expect(cf.Cf("push", appName, "--no-start", "-m", DEFAULT_MEMORY_LIMIT, "-p", appPath, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		app_helpers.SetBackend(appName)

		start := cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)
		Expect(start).To(Exit(1))
		Expect(start).To(Say("NoAppDetectedError"))
	}

	itDoesNotDetectWhenBuildpackDeleted := func() {
		cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
			Expect(cf.Cf("delete-buildpack", BuildpackName, "-f").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		})
		Expect(cf.Cf("push", appName, "--no-start", "-m", DEFAULT_MEMORY_LIMIT, "-p", appPath, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
		app_helpers.SetBackend(appName)

		start := cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)
		Expect(start).To(Exit(1))
		Expect(start).To(Say("NoAppDetectedError"))
	}

	Context("when the buildpack is not specified", func() {
		It("runs the app only if the buildpack is detected", func() {
			// Tests that rely on buildpack detection must be run in serial,
			// but ginkgo doesn't allow specific blocks to be marked as serial-only
			// so we manually mimic setup/teardown pattern here

			setupBuildpack(appConfig{Empty: false})
			itIsUsedForTheApp()
			deleteApp()
			deleteBuildpack()

			setupBuildpack(appConfig{Empty: true})
			itDoesNotDetectForEmptyApp()
			deleteApp()
			deleteBuildpack()

			setupBuildpack(appConfig{Empty: false})
			itDoesNotDetectWhenBuildpackDisabled()
			deleteApp()
			deleteBuildpack()

			setupBuildpack(appConfig{Empty: false})
			itDoesNotDetectWhenBuildpackDeleted()
			deleteApp()
			deleteBuildpack()
		})
	})
})
