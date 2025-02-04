/*
Copyright The Kubernetes NMState Authors.


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

package handler

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	nmstate "github.com/nmstate/kubernetes-nmstate/api/shared"
	enactmentconditions "github.com/nmstate/kubernetes-nmstate/pkg/enactmentstatus/conditions"
	policyconditions "github.com/nmstate/kubernetes-nmstate/test/e2e/policy"
)

func createInterfaceWithNonExistingCapture() nmstate.State {
	return nmstate.NewState(`interfaces:
  - name: "{{ capture.base-iface.interfaces.0.name }}"
    type: ethernet
    state: up`)
}

func createInterfaceWithMismatchedName() nmstate.State {
	return nmstate.NewState(`interfaces:
  - name: eth666
    type: ethernet
    state: up`)
}

func createInterfaceWithInvalidField() nmstate.State {
	return nmstate.NewState(fmt.Sprintf(`interfaces:
  - name: %s
    type: ethernet
    invalid_state: up`, primaryNic))
}

func createInterfaceWithIncorrectIP() nmstate.State {
	return nmstate.NewState(fmt.Sprintf(`interfaces:
  - name: %s
    type: ethernet
    state: up
    ipv4:
      address:
      - ip: "192.168.45.33"
        prefix-length: 24
      dhcp: false
      enabled: true`, primaryNic))
}

func createPolicyAndWaitForEnactmentCondition(policy string, desiredState func() nmstate.State, nodeHostname string) {
	By("Creating the policy")
	err := setDesiredStateWithPolicyAndNodeSelector(policy, desiredState(), map[string]string{"kubernetes.io/hostname": nodeHostname})
	if err != nil {
		return
	}

	By("Waiting until the node becomes ready again")
	waitForNodesReady()

	By("Waiting for enactment to be failing")
	policyconditions.EnactmentConditionsStatusForPolicyEventually(
		nodes[0],
		policy,
	).Should(policyconditions.MatchConditionsFrom(enactmentconditions.SetFailedToConfigure))
}

var _ = Describe("NodeNetworkState", func() {
	var (
		defaultPolicy = "errors-policy"

		messagesToRemove = []string{
			"DEBUG    Async action: Create checkpoint started",
			"DEBUG    Checkpoint None created for all devices",
			"Traceback (most recent call last):",
			"DEBUG    Nispor: current network state",
			"WARNING  libnm version",
			"rolling back desired state configuration: failed running probes after network changes: ",
			"failed running probe 'ping' with after network reconfiguration -> currentState:",
			"warnings.warn",
		}
	)

	Context("with invalid field", func() {
		var (
			messagesToKeep = []string{
				"libnmstate.error.NmstateVerificationError",
				"desired",
				"current",
				"difference",
			}
		)

		BeforeEach(func() {
			createPolicyAndWaitForEnactmentCondition(defaultPolicy, createInterfaceWithInvalidField, nodes[0])
		})

		It("should discard disarranged parts of the message", func() {
			for _, unwantedMessage := range messagesToRemove {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).NotTo(ContainSubstring(unwantedMessage))
			}
		})

		It("should keep desired parts of the message", func() {
			for _, desiredMessage := range messagesToKeep {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).To(ContainSubstring(desiredMessage))
			}
		})
	})

	Context("with mismatched interface name", func() {
		var (
			messagesToKeep = []string{
				"libnmstate.error.NmstateLibnmError",
				"No suitable device found for this connection",
				"mismatching interface name",
			}
		)

		BeforeEach(func() {
			createPolicyAndWaitForEnactmentCondition(defaultPolicy, createInterfaceWithMismatchedName, nodes[0])
		})

		It("should discard disarranged parts of the message", func() {
			for _, unwantedMessage := range messagesToRemove {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).NotTo(ContainSubstring(unwantedMessage))
			}
		})

		It("should keep desired parts of the message", func() {
			for _, desiredMessage := range messagesToKeep {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).To(ContainSubstring(desiredMessage))
			}
		})
	})

	Context("with ping fail", func() {
		var (
			messagesToKeep = []string{
				"timed out waiting for the condition",
			}
		)

		BeforeEach(func() {
			createPolicyAndWaitForEnactmentCondition(defaultPolicy, createInterfaceWithIncorrectIP, nodes[0])
		})

		AfterEach(func() {
			resetDesiredStateForNodes()
			By("Remove the policy")
			deletePolicy(defaultPolicy)
		})

		It("should discard disarranged parts of the message and keep desired parts of the message", func() {
			for _, unwantedMessage := range messagesToRemove {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).NotTo(ContainSubstring(unwantedMessage))
			}
			for _, desiredMessage := range messagesToKeep {
				Expect(
					policyconditions.EnactmentConditionsStatus(
						nodes[0],
						defaultPolicy,
					).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).
						Message,
				).To(ContainSubstring(desiredMessage))
			}
		})
	})

	Context("with non existing capture", func() {
		BeforeEach(func() {
			createPolicyAndWaitForEnactmentCondition(defaultPolicy, createInterfaceWithNonExistingCapture, nodes[0])
		})

		AfterEach(func() {
			By("Remove the policy")
			deletePolicy(defaultPolicy)
		})

		It("should contain the error message", func() {
			Expect(
				policyconditions.EnactmentConditionsStatus(
					nodes[0], defaultPolicy,
				).Find(nmstate.NodeNetworkConfigurationEnactmentConditionFailing).Message,
			).To(ContainSubstring("failure generating desiredState and capturedStates"))
		})
	})
})
