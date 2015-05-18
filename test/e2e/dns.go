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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/latest"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/wait"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var dnsServiceLableSelector = labels.Set{
	"k8s-app":                       "kube-dns",
	"kubernetes.io/cluster-service": "true",
}.AsSelector()

var _ = Describe("DNS", func() {
	var c *client.Client
	// Use this in tests.  They're unique for each test to prevent name collisions.
	var testNamespace string

	BeforeEach(func() {
		var err error
		c, err = loadClient()
		Expect(err).NotTo(HaveOccurred())
		ns, err := createTestingNS("dns", c)
		Expect(err).NotTo(HaveOccurred())
		testNamespace = ns.Name
	})
	It("should provide DNS for the cluster", func() {
		if providerIs("vagrant") {
			By("Skipping test which is broken for vagrant (See https://github.com/GoogleCloudPlatform/kubernetes/issues/3580)")
			return
		}

		podClient := c.Pods(api.NamespaceDefault)

		By("Waiting for DNS Service to be Running")
		dnsPods, err := podClient.List(dnsServiceLableSelector, fields.Everything())
		if err != nil {
			Failf("Failed to list all dns service pods")
		}
		if len(dnsPods.Items) != 1 {
			Failf("Unexpected number of pods (%d) matches the label selector %v", len(dnsPods.Items), dnsServiceLableSelector.String())
		}
		expectNoError(waitForPodRunning(c, dnsPods.Items[0].Name))

		// All the names we need to be able to resolve.
		// TODO: Spin up a separate test service and test that dns works for that service.
		namesToResolve := []string{
			"kubernetes-ro.default",
			"kubernetes-ro.default.cluster.local",
			"google.com",
		}

		probeCmd := "for i in `seq 1 600`; do "
		for _, name := range namesToResolve {
			// Resolve by TCP and UDP DNS.
			probeCmd += fmt.Sprintf(`test -n "$(dig +notcp +noall +answer +search %s)" && echo OK > /results/udp@%s;`, name, name)
			probeCmd += fmt.Sprintf(`test -n "$(dig +tcp +noall +answer +search %s)" && echo OK > /results/tcp@%s;`, name, name)
		}
		probeCmd += "sleep 1; done"

		// Run a pod which probes DNS and exposes the results by HTTP.
		By("creating a pod to probe DNS")
		pod := &api.Pod{
			TypeMeta: api.TypeMeta{
				Kind:       "Pod",
				APIVersion: latest.Version,
			},
			ObjectMeta: api.ObjectMeta{
				Name:      "dns-test-" + string(util.NewUUID()),
				Namespace: testNamespace,
			},
			Spec: api.PodSpec{
				Volumes: []api.Volume{
					{
						Name: "results",
						VolumeSource: api.VolumeSource{
							EmptyDir: &api.EmptyDirVolumeSource{},
						},
					},
				},
				Containers: []api.Container{
					// TODO: Consider scraping logs instead of running a webserver.
					{
						Name:  "webserver",
						Image: "gcr.io/google_containers/test-webserver",
						VolumeMounts: []api.VolumeMount{
							{
								Name:      "results",
								MountPath: "/results",
							},
						},
					},
					{
						Name:    "querier",
						Image:   "gcr.io/google_containers/dnsutils",
						Command: []string{"sh", "-c", probeCmd},
						VolumeMounts: []api.VolumeMount{
							{
								Name:      "results",
								MountPath: "/results",
							},
						},
					},
				},
			},
		}

		By("submitting the pod to kubernetes")
		podClient = c.Pods(testNamespace)
		defer func() {
			By("deleting the pod")
			defer GinkgoRecover()
			podClient.Delete(pod.Name, nil)
		}()
		if _, err := podClient.Create(pod); err != nil {
			Failf("Failed to create %s pod: %v", pod.Name, err)
		}

		expectNoError(waitForPodRunningInNamespace(c, pod.Name, testNamespace))

		By("retrieving the pod")
		pod, err = podClient.Get(pod.Name)
		if err != nil {
			Failf("Failed to get pod %s: %v", pod.Name, err)
		}

		// Try to find results for each expected name.
		By("looking for the results for each expected name")
		var failed []string

		expectNoError(wait.Poll(time.Second*2, time.Second*60, func() (bool, error) {
			failed = []string{}
			for _, name := range namesToResolve {
				for _, proto := range []string{"udp", "tcp"} {
					testCase := fmt.Sprintf("%s@%s", proto, name)
					_, err := c.Get().
						Prefix("proxy").
						Resource("pods").
						Namespace(testNamespace).
						Name(pod.Name).
						Suffix("results", testCase).
						Do().Raw()
					if err != nil {
						failed = append(failed, testCase)
					}
				}
			}
			if len(failed) == 0 {
				return true, nil
			}
			Logf("Lookups using %s failed for: %v\n", pod.Name, failed)
			return false, nil
		}))
		Expect(len(failed)).To(Equal(0))

		// TODO: probe from the host, too.

		Logf("DNS probes using %s succeeded\n", pod.Name)
	})
})
