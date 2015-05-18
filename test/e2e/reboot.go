/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	// How long to pause between polling node or pod status.
	poll = 5 * time.Second

	// How long nodes have to be "ready" before the reboot. They should already
	// be "ready" before the test starts, so this is small.
	nodeReadyInitialTimeout = 20 * time.Second

	// How long pods have to be "ready" before the reboot. They should already
	// be "ready" before the test starts, so this is small.
	podReadyBeforeTimeout = 20 * time.Second

	// How long a node is allowed to go from "Ready" to "NotReady" after a
	// reboot is issued before the test is considered failed.
	rebootNotReadyTimeout = 2 * time.Minute

	// How long a node is allowed to go from "NotReady" to "Ready" after a
	// reboot is issued and it is found to be "NotReady" before the test is
	// considered failed.
	rebootReadyAgainTimeout = 5 * time.Minute

	// How long pods have to be "ready" after the reboot.
	podReadyAgainTimeout = 5 * time.Minute
)

var _ = Describe("Reboot", func() {
	BeforeEach(func() {
		var err error
		c, err = loadClient()
		Expect(err).NotTo(HaveOccurred())
	})

	It("should reboot each node and ensure they function upon restart", func() {
		// This test requires SSH, so the provider check should be identical to
		// there (the limiting factor is the implementation of util.go's
		// getSigner(...)).
		provider := testContext.Provider
		if !providerIs("gce", "gke") {
			By(fmt.Sprintf("Skipping reboot test, which is not implemented for %s", provider))
			return
		}

		// Get all nodes, and kick off the test on each.
		nodelist, err := c.Nodes().List(labels.Everything(), fields.Everything())
		if err != nil {
			Failf("Error getting nodes: %v", err)
		}
		result := make(chan bool, len(nodelist.Items))
		for _, n := range nodelist.Items {
			go rebootNode(c, provider, n.ObjectMeta.Name, result)
		}

		// Wait for all to finish and check the final result.
		failed := false
		// TODO(mbforbes): Change to `for range` syntax and remove logging once
		// we support only Go >= 1.4.
		for _, n := range nodelist.Items {
			if !<-result {
				Failf("Node %s failed reboot test.", n.ObjectMeta.Name)
				failed = true
			}
		}
		if failed {
			Failf("Test failed; at least one node failed to reboot in the time given.")
		}
	})
})

// rebootNode takes node name on provider through the following steps using c:
//  - ensures the node is ready
//  - ensures all pods on the node are running and ready
//  - reboots the node
//  - ensures the node reaches some non-ready state
//  - ensures the node becomes ready again
//  - ensures all pods on the node become running and ready again
//
// It returns true through result only if all of the steps pass; at the first
// failed step, it will return false through result and not run the rest.
func rebootNode(c *client.Client, provider, name string, result chan bool) {
	// Get the node initially.
	Logf("Getting %s", name)
	node, err := c.Nodes().Get(name)
	if err != nil {
		Logf("Couldn't get node %s", name)
		result <- false
		return
	}

	// Node sanity check: ensure it is "ready".
	if !waitForNodeToBeReady(c, name, nodeReadyInitialTimeout) {
		result <- false
		return
	}

	// Get all the pods on the node.
	podList, err := c.Pods(api.NamespaceDefault).List(
		labels.Everything(), fields.OneTermEqualSelector(client.PodHost, name))
	if err != nil {
		Logf("Error getting pods for node %s: %v", name, err)
		result <- false
		return
	}
	podNames := make([]string, len(podList.Items))
	for i, p := range podList.Items {
		podNames[i] = p.ObjectMeta.Name
	}
	Logf("Node %s has %d pods: %v", name, len(podNames), podNames)

	// For each pod, we do a sanity check to ensure it's running / healthy
	// now, as that's what we'll be checking later.
	if !checkPodsRunning(c, podNames, podReadyBeforeTimeout) {
		result <- false
		return
	}

	// Reboot the node.
	Logf("Getting external IP address for %s", name)
	host := ""
	for _, a := range node.Status.Addresses {
		if a.Type == api.NodeExternalIP {
			host = a.Address + ":22"
			break
		}
	}
	if host == "" {
		Logf("Couldn't find external IP address for node %s", name)
		result <- false
		return
	}
	Logf("Calling reboot on %s", name)
	rebootCmd := "sudo reboot"
	if _, _, code, err := SSH(rebootCmd, host, provider); code != 0 || err != nil {
		Failf("Expected 0 exit code and nil error when running %s on %s, got %d and %v",
			rebootCmd, node, code, err)
		result <- false
		return
	}

	// Wait for some kind of "not ready" status.
	if !waitForNodeToBeNotReady(c, name, rebootNotReadyTimeout) {
		result <- false
		return
	}

	// Wait for some kind of "ready" status.
	if !waitForNodeToBeReady(c, name, rebootReadyAgainTimeout) {
		result <- false
		return
	}

	// Ensure all of the pods that we found on this node before the reboot are
	// running / healthy.
	if !checkPodsRunning(c, podNames, podReadyAgainTimeout) {
		result <- false
		return
	}

	Logf("Reboot successful on node %s", name)
	result <- true
}

// podRunningReady is the checker function passed to waitForPodCondition(...)
// (found in util.go). It ensures that the pods' phase is running and that the
// ready condition is true.
func podRunningReady(p *api.Pod) (bool, error) {
	// Check the phase is running.
	if p.Status.Phase != api.PodRunning {
		return false, fmt.Errorf("want pod %s on %s to be %v but was %v",
			p.ObjectMeta.Name, p.Spec.Host, api.PodRunning, p.Status.Phase)
	}
	// Check the ready condition is true.
	for _, cond := range p.Status.Conditions {
		if cond.Type == api.PodReady && cond.Status == api.ConditionTrue {
			return true, nil
		}
	}
	return false, fmt.Errorf("pod %s on %s didn't have condition %v, %v; conditions: %v",
		p.ObjectMeta.Name, p.Spec.Host, api.PodReady, api.ConditionTrue, p.Status.Conditions)
}

// checkPodsRunning returns whether all pods whose names are listed in podNames
// are running.
func checkPodsRunning(c *client.Client, podNames []string, timeout time.Duration) bool {
	desc := "running and ready"
	Logf("Waiting up to %v for the following pods to be %s: %s", timeout, desc, podNames)
	result := make(chan bool, len(podNames))
	for ix := range podNames {
		// Launch off pod readiness checkers.
		go func(name string) {
			err := waitForPodCondition(c, api.NamespaceDefault, name, desc,
				poll, timeout, podRunningReady)
			result <- err == nil
		}(podNames[ix])
	}
	// Wait for them all to finish.
	success := true
	// TODO(mbforbes): Change to `for range` syntax and remove logging once we
	// support only Go >= 1.4.
	for _, podName := range podNames {
		if !<-result {
			Logf("Pod %s failed to be %s.", podName, desc)
			success = false
		}
	}
	Logf("Wanted all pods to be %s. Result: %t. Pods: %v", desc, success, podNames)
	return success
}

// waitForNodeToBeReady returns whether node name is ready within timeout.
func waitForNodeToBeReady(c *client.Client, name string, timeout time.Duration) bool {
	return waitForNodeToBe(c, name, true, timeout)
}

// waitForNodeToBeNotReady returns whether node name is not ready (i.e. the
// readiness condition is anything but ready, e.g false or unknown) within
// timeout.
func waitForNodeToBeNotReady(c *client.Client, name string, timeout time.Duration) bool {
	return waitForNodeToBe(c, name, false, timeout)
}

// waitForNodeToBe returns whether node name's readiness state matches wantReady
// within timeout. If wantReady is true, it will ensure the node is ready; if
// it's false, it ensures the node is in any state other than ready (e.g. not
// ready or unknown).
func waitForNodeToBe(c *client.Client, name string, wantReady bool, timeout time.Duration) bool {
	Logf("Waiting up to %v for node %s readiness to be %t", timeout, name, wantReady)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(poll) {
		node, err := c.Nodes().Get(name)
		if err != nil {
			Logf("Couldn't get node %s", name)
			continue
		}

		// Check the node readiness condition (logging all).
		for i, cond := range node.Status.Conditions {
			Logf("Node %s condition %d/%d: type: %v, status: %v",
				name, i+1, len(node.Status.Conditions), cond.Type, cond.Status)
			// Ensure that the condition type is readiness and the status
			// matches as desired.
			if cond.Type == api.NodeReady && (cond.Status == api.ConditionTrue) == wantReady {
				Logf("Successfully found node %s readiness to be %t", name, wantReady)
				return true
			}
		}
	}
	Logf("Node %s didn't reach desired readiness (%t) within %v", name, wantReady, timeout)
	return false
}
