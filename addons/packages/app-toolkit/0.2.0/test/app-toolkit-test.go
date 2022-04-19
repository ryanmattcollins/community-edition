// Copyright 2022 VMware Tanzu Community Edition contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

const workloadURL = "http://tanzu-simple-web-app.test-namespace.127-0-0-1.sslip.io/"

func main() {
	fmt.Println("STARTING TEST")

	if checkCommand("tanzu uc list", "app-toolkit-test") {
		runCommand("tanzu uc delete app-toolkit-test")
	}

	fmt.Println("\nTEST STEP Executable check")
	validateCommand("tanzu", "Tanzu CLI")
	validateCommand("tanzu apps", "Applications on Kubernetes")
	validateCommand("tanzu secret", "Tanzu secret management")
	validateCommand("tanzu package", "Tanzu package management")
	validateCommand("kubectl", "kubectl controls the Kubernetes cluster manager")
	validateCommand("docker", "A self-sufficient runtime for containers")
	fmt.Println("TEST STEP Executable check OK")

	fmt.Println("\nTEST STEP Install TCE and wait for ready")
	runCommand("tanzu uc create app-toolkit-test -p 80:80 -p 443:443")
	pollCommand("kubectl get nodes", " Ready", 10)
	fmt.Println("TEST STEP Install TCE and wait for ready OK")

	fmt.Println("\nTEST STEP Prepare Secrets")
	//TODO: remove when we can use the official package repository
	runCommand("tanzu package repository update projects.registry.vmware.com-tce-main-v0.11.0 -n tanzu-package-repo-global --url index.docker.io/ryanmattcollins/main@sha256:52da5141d10490fa755fb90e56141653160d8af2a9c0b3a37bf1acfd802cfc8b")
	runCommand("tanzu package install secretgen-controller --package-name secretgen-controller.community.tanzu.vmware.com --version 0.8.0")
	runCommand("/bin/bash setup_registry_secret.sh")
	validateCommand("tanzu secret registry list", "registry-credentials")
	fmt.Println("TEST STEP Prepare Secrets")

	fmt.Println("\nTEST STEP Install app-toolkit")
	runCommand("tanzu package install app-toolkit -p app-toolkit.community.tanzu.vmware.com -v 0.2.0 -n tanzu-package-repo-global -f app-toolkit-values.yaml")
	runCommand("/bin/bash wait_for_app_toolkit.sh")
	fmt.Println("TEST STEP Install app-toolkit OK")

	fmt.Println("\nTEST STEP Install workload")
	runCommand("tanzu apps workload create --yes -f workload.yaml")
	watchCommand("tanzu apps workload tail tanzu-simple-web-app", "Build successful")
	fmt.Println("TEST STEP Install workload OK")
	fmt.Println("\nTEST STEP Call workload")
	checkWorkload()
	fmt.Println("\nTEST STEP Call workload OK")
	fmt.Println("\nALL TESTS PASS OK")
}

func checkWorkload() {
	ready := false
	waitInterval := 5
	var resp *http.Response
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", workloadURL, http.NoBody)
	client := &http.Client{}

	for !ready {
		var err error
		resp, err = client.Do(req)
		if err != nil {
			fmt.Printf("Waiting %d sec for workload ready...\n", waitInterval)
			resp.Body.Close()
			time.Sleep(time.Duration(waitInterval) * time.Second)
		} else {
			if resp.StatusCode != 200 {
				fmt.Printf("Waiting %d sec for workload ready...\n", waitInterval)
				time.Sleep(time.Duration(waitInterval) * time.Second)
				resp.Body.Close()
			} else {
				ready = true
			}
		}
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s\n", body)
}

func runCommand(commandString string) string {
	return watchCommand(commandString, "")
}

func validateCommand(commandString, checkFor string) {
	output := watchCommand(commandString, "")
	if !strings.Contains(output, checkFor) {
		panic(fmt.Sprintf("Not found in return: %s", checkFor))
	}
}

func checkCommand(commandString, checkFor string) bool {
	output := watchCommand(commandString, "")
	return strings.Contains(output, checkFor)
}

func pollCommand(commandString, polledFor string, pollInterval int) {
	ready := false
	for !ready {
		output := runCommand(commandString)
		if strings.Contains(output, polledFor) {
			ready = true
		} else {
			time.Sleep(time.Duration(pollInterval) * time.Second)
		}
	}
}

func watchCommand(commandString, watched string) string {
	words := strings.Fields(commandString)
	executable, err := exec.LookPath(words[0])
	if err != nil {
		panic(fmt.Sprintf("Executable not found: %s", executable))
	}
	cmd := &exec.Cmd{
		Path: executable,
		Args: words,
	}

	fmt.Printf("%s\n", cmd.String())

	f, err := pty.Start(cmd)
	if err != nil {
		panic(fmt.Sprintf("START, AN ERROR: %s", err))
	}

	done := make(chan struct{})
	scanner := bufio.NewScanner(f)
	var output bytes.Buffer
	go func() {
		for scanner.Scan() {
			line := scanner.Bytes()
			output.WriteString(string(line))
			fmt.Printf("%s\n", line)

			if len(watched) > 0 {
				if strings.Contains(string(line), watched) {
					fmt.Printf("BREAK\n")
					_ = cmd.Process.Kill()
					break
				}
			}
		}
		done <- struct{}{}
	}()

	<-done

	err = cmd.Wait()
	if err != nil {
		if !strings.Contains(output.String(), "TLS handshake timeout") &&
			!strings.Contains(err.Error(), "signal: killed") {
			panic(fmt.Sprintf("WAIT, AN ERROR: %s", err))
		}
	}

	return output.String()
}
