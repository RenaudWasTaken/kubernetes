/*
Copyright 2017 The Kubernetes Authors.

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

package solarflare

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/api/v1"
)

type testActivePodsLister struct {
	activePods []*v1.Pod
}

func (tapl *testActivePodsLister) GetActivePods() []*v1.Pod {
	return tapl.activePods
}

func makeTestPod(numContainers, nicsPerContainer int) *v1.Pod {
	quantity := resource.NewQuantity(int64(nicsPerContainer), resource.DecimalSI)
	resources := v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceSolarflareNIC: *quantity,
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{},
		},
	}
	for ; numContainers > 0; numContainers-- {
		pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
			Name:      string(uuid.NewUUID()),
			Resources: resources,
		})
	}
	return pod
}

func TestMultiContainerPodNICAllocation(t *testing.T) {
	podLister := &testActivePodsLister{}

	testNicManager := &solarflareNICManager{
		activePodsLister: podLister,
		allNICs:          sets.NewString("/dev/solarflare0", "/dev/solarflare1"),
		allocated:        newPodNICs(),
	}

	// Expect that no devices are in use.
	nicsInUse := testNicManager.nicsInUse()
	as := assert.New(t)
	as.Equal(len(nicsInUse.devices()), 0)

	// Allocated NICs for a pod with two containers.
	pod := makeTestPod(2, 1)
	// Allocate for the first container.
	devices1, err := testNicManager.AllocateNIC(pod, &pod.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devices1), 1)

	podLister.activePods = append(podLister.activePods, pod)
	// Allocate for the second container.
	devices2, err := testNicManager.AllocateNIC(pod, &pod.Spec.Containers[1])
	as.Nil(err)
	as.Equal(len(devices2), 1)

	as.NotEqual(devices1, devices2, "expected containers to get different devices")

	// further allocations should fail.
	newPod := makeTestPod(2, 1)
	devices1, err = testNicManager.AllocateNIC(newPod, &newPod.Spec.Containers[0])
	as.NotNil(err, "expected nic allocation to fail. got: %v", devices1)

	// Now terminate the original pod and observe that NIC allocation for new pod succeeds.
	podLister.activePods = podLister.activePods[:0]

	devices1, err = testNicManager.AllocateNIC(newPod, &newPod.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devices1), 1)

	podLister.activePods = append(podLister.activePods, newPod)

	devices2, err = testNicManager.AllocateNIC(newPod, &newPod.Spec.Containers[1])
	as.Nil(err)
	as.Equal(len(devices2), 1)

	as.NotEqual(devices1, devices2, "expected containers to get different devices")
}

func TestMultiPodNICAllocation(t *testing.T) {
	podLister := &testActivePodsLister{}

	testNicManager := &solarflareNICManager{
		activePodsLister: podLister,
		allNICs:          sets.NewString("/dev/solarflare0", "/dev/solarflare1"),
		allocated:        newPodNICs(),
	}

	// Expect that no devices are in use.
	nicsInUse := testNicManager.nicsInUse()
	as := assert.New(t)
	as.Equal(len(nicsInUse.devices()), 0)

	// Allocated NICs for a pod with two containers.
	podA := makeTestPod(1, 1)
	// Allocate for the first container.
	devicesA, err := testNicManager.AllocateNIC(podA, &podA.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devicesA), 1)

	podLister.activePods = append(podLister.activePods, podA)

	// further allocations should fail.
	podB := makeTestPod(1, 1)
	// Allocate for the first container.
	devicesB, err := testNicManager.AllocateNIC(podB, &podB.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devicesB), 1)
	as.NotEqual(devicesA, devicesB, "expected pods to get different devices")
}

func TestPodContainerRestart(t *testing.T) {
	podLister := &testActivePodsLister{}

	testNicManager := &solarflareNICManager{
		activePodsLister: podLister,
		allNICs:          sets.NewString("/dev/solarflare0", "/dev/solarflare1"),
		allocated:        newPodNICs(),
		defaultDevices:   []string{"/dev/solarflare-smi"},
	}

	// Expect that no devices are in use.
	nicsInUse := testNicManager.nicsInUse()
	as := assert.New(t)
	as.Equal(len(nicsInUse.devices()), 0)

	// Make a pod with one containers that requests two NICs.
	podA := makeTestPod(1, 2)
	// Allocate NICs
	devicesA, err := testNicManager.AllocateNIC(podA, &podA.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devicesA), 3)

	podLister.activePods = append(podLister.activePods, podA)

	// further allocations should fail.
	podB := makeTestPod(1, 1)
	_, err = testNicManager.AllocateNIC(podB, &podB.Spec.Containers[0])
	as.NotNil(err)

	// Allcate NIC for existing Pod A.
	// The same nics must be returned.
	devicesAretry, err := testNicManager.AllocateNIC(podA, &podA.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(devicesA), 3)
	as.True(sets.NewString(devicesA...).Equal(sets.NewString(devicesAretry...)))
}
