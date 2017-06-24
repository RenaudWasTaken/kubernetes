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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/libdocker"
	"k8s.io/kubernetes/pkg/kubelet/nic"
)

// TODO: rework to use solarflare's NVML, which is more complex, but also provides more fine-grained information and stats.
const (
	// All solarflare NICs cards should be mounted with solarflarectl and solarflare-uvm
	// If the driver installed correctly, the 2 devices will be there.
	solarflareCtlDevice string = "/dev/solarflarectl"
	solarflareUVMDevice string = "/dev/solarflare-uvm"
	// Optional device.
	solarflareUVMToolsDevice string = "/dev/solarflare-uvm-tools"
	devDirectory                = "/dev"
	solarflareDeviceRE              = `^solarflare[0-9]*$`
	solarflareFullpathRE            = `^/dev/solarflare[0-9]*$`
)

type activePodsLister interface {
	// Returns a list of active pods on the node.
	GetActivePods() []*v1.Pod
}

// solarflareNICManager manages solarflare nic devices.
type solarflareNICManager struct {
	sync.Mutex
	// All nics available on the Node
	allNICs        sets.String
	allocated      *podNICs
	defaultDevices []string
	// The interface which could get NIC mapping from all the containers.
	// TODO: Should make this independent of Docker in the future.
	dockerClient     libdocker.Interface
	activePodsLister activePodsLister
}

// NewSolarflareNICManager returns a NICManager that manages local solarflare NICs.
// TODO: Migrate to use pod level cgroups and make it generic to all runtimes.
func NewSolarflareNICManager(activePodsLister activePodsLister, dockerClient libdocker.Interface) (nic.NICManager, error) {
	if dockerClient == nil {
		return nil, fmt.Errorf("invalid docker client specified")
	}
	return &solarflareNICManager{
		allNICs:          sets.NewString(),
		dockerClient:     dockerClient,
		activePodsLister: activePodsLister,
	}, nil
}

// Initialize the NIC devices, so far only needed to discover the NIC paths.
func (ngm *solarflareNICManager) Start() error {
	if ngm.dockerClient == nil {
		return fmt.Errorf("Invalid docker client specified in NIC Manager")
	}
	ngm.Lock()
	defer ngm.Unlock()

	if _, err := os.Stat(solarflareCtlDevice); err != nil {
		return err
	}

	if _, err := os.Stat(solarflareUVMDevice); err != nil {
		return err
	}
	ngm.defaultDevices = []string{solarflareCtlDevice, solarflareUVMDevice}
	_, err := os.Stat(solarflareUVMToolsDevice)
	if !os.IsNotExist(err) {
		ngm.defaultDevices = append(ngm.defaultDevices, solarflareUVMToolsDevice)
	}

	if err := ngm.discoverNICs(); err != nil {
		return err
	}

	// We ignore errors when identifying allocated NICs because it is possible that the runtime interfaces may be not be logically up.
	return nil
}

// Get how many NIC cards we have.
func (ngm *solarflareNICManager) Capacity() v1.ResourceList {
	nics := resource.NewQuantity(int64(len(ngm.allNICs)), resource.DecimalSI)
	return v1.ResourceList{
		v1.ResourceSolarflareNIC: *nics,
	}
}

// AllocateNICs returns `num` NICs if available, error otherwise.
// Allocation is made thread safe using the following logic.
// A list of all NICs allocated is maintained along with their respective Pod UIDs.
// It is expected that the list of active pods will not return any false positives.
// As part of initialization or allocation, the list of NICs in use will be computed once.
// Whenever an allocation happens, the list of NICs allocated is updated based on the list of currently active pods.
// NICs allocated to terminated pods are freed up lazily as part of allocation.
// NICs are allocated based on the internal list of allocatedNICs.
// It is not safe to generate a list of NICs in use by inspecting active containers because of the delay between NIC allocation and container creation.
// A NIC allocated to a container might be re-allocated to a subsequent container because the original container wasn't started quick enough.
// The current algorithm scans containers only once and then uses a list of active pods to track NIC usage.
// This is a sub-optimal solution and a better alternative would be that of using pod level cgroups instead.
// NICs allocated to containers should be reflected in pod level device cgroups before completing allocations.
// The pod level cgroups will then serve as a checkpoint of NICs in use.
func (ngm *solarflareNICManager) AllocateNIC(pod *v1.Pod, container *v1.Container) ([]string, error) {
	nicsNeeded := container.Resources.Limits.SolarflareNIC().Value()
	if nicsNeeded == 0 {
		return []string{}, nil
	}
	ngm.Lock()
	defer ngm.Unlock()
	if ngm.allocated == nil {
		// Initialization is not complete. Try now. Failures can no longer be tolerated.
		ngm.allocated = ngm.nicsInUse()
	} else {
		// update internal list of NICs in use prior to allocating new NICs.
		ngm.updateAllocatedNICs()
	}
	// Check if NICs have already been allocated. If so return them right away.
	// This can happen if a container restarts for example.
	if devices := ngm.allocated.getNICs(string(pod.UID), container.Name); devices != nil {
		glog.V(2).Infof("Found pre-allocated NICs for container %q in Pod %q: %v", container.Name, pod.UID, devices.List())
		return append(devices.List(), ngm.defaultDevices...), nil
	}
	// Get NIC devices in use.
	devicesInUse := ngm.allocated.devices()
	glog.V(5).Infof("nics in use: %v", devicesInUse.List())
	// Get a list of available NICs.
	available := ngm.allNICs.Difference(devicesInUse)
	glog.V(5).Infof("nics available: %v", available.List())
	if int64(available.Len()) < nicsNeeded {
		return nil, fmt.Errorf("requested number of NICs unavailable. Requested: %d, Available: %d", nicsNeeded, available.Len())
	}
	ret := available.UnsortedList()[:nicsNeeded]
	for _, device := range ret {
		// Update internal allocated NIC cache.
		ngm.allocated.insert(string(pod.UID), container.Name, device)
	}
	// Add standard devices files that needs to be exposed.
	ret = append(ret, ngm.defaultDevices...)

	return ret, nil
}

// updateAllocatedNICs updates the list of NICs in use.
// It gets a list of active pods and then frees any NICs that are bound to terminated pods.
// Returns error on failure.
func (ngm *solarflareNICManager) updateAllocatedNICs() {
	activePods := ngm.activePodsLister.GetActivePods()
	activePodUids := sets.NewString()
	for _, pod := range activePods {
		activePodUids.Insert(string(pod.UID))
	}
	allocatedPodUids := ngm.allocated.pods()
	podsToBeRemoved := allocatedPodUids.Difference(activePodUids)
	glog.V(5).Infof("pods to be removed: %v", podsToBeRemoved.List())
	ngm.allocated.delete(podsToBeRemoved.List())
}

// discoverNICs identifies allNICs solarflare NIC devices available on the local node by walking `/dev` directory.
// TODO: Without NVML support we only can check whether there has NIC devices, but
// could not give a health check or get more information like NIC cores, memory, or
// family name. Need to support NVML in the future. But we do not need NVML until
// we want more features, features like schedule containers according to NIC family
// name.
func (ngm *solarflareNICManager) discoverNICs() error {
	reg := regexp.MustCompile(solarflareDeviceRE)
	files, err := ioutil.ReadDir(devDirectory)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if reg.MatchString(f.Name()) {
			glog.V(2).Infof("Found solarflare NIC %q", f.Name())
			ngm.allNICs.Insert(path.Join(devDirectory, f.Name()))
		}
	}

	return nil
}

// nicsInUse returns a list of NICs in use along with the respective pods that are using it.
func (ngm *solarflareNICManager) nicsInUse() *podNICs {
	pods := ngm.activePodsLister.GetActivePods()
	type containerIdentifier struct {
		id   string
		name string
	}
	type podContainers struct {
		uid        string
		containers []containerIdentifier
	}
	// List of containers to inspect.
	podContainersToInspect := []podContainers{}
	for _, pod := range pods {
		containers := sets.NewString()
		for _, container := range pod.Spec.Containers {
			// NICs are expected to be specified only in limits.
			if !container.Resources.Limits.SolarflareNIC().IsZero() {
				containers.Insert(container.Name)
			}
		}
		// If no NICs were requested skip this pod.
		if containers.Len() == 0 {
			continue
		}
		// TODO: If kubelet restarts right after allocating a NIC to a pod, the container might not have started yet and so container status might not be available yet.
		// Use an internal checkpoint instead or try using the CRI if its checkpoint is reliable.
		var containersToInspect []containerIdentifier
		for _, container := range pod.Status.ContainerStatuses {
			if containers.Has(container.Name) {
				containersToInspect = append(containersToInspect, containerIdentifier{strings.Replace(container.ContainerID, "docker://", "", 1), container.Name})
			}
		}
		// add the pod and its containers that need to be inspected.
		podContainersToInspect = append(podContainersToInspect, podContainers{string(pod.UID), containersToInspect})
	}
	ret := newPodNICs()
	for _, podContainer := range podContainersToInspect {
		for _, containerIdentifier := range podContainer.containers {
			containerJSON, err := ngm.dockerClient.InspectContainer(containerIdentifier.id)
			if err != nil {
				glog.V(3).Infof("Failed to inspect container %q in pod %q while attempting to reconcile solarflare nics in use", containerIdentifier.id, podContainer.uid)
				continue
			}

			devices := containerJSON.HostConfig.Devices
			if devices == nil {
				continue
			}

			for _, device := range devices {
				if isValidPath(device.PathOnHost) {
					glog.V(4).Infof("solarflare NIC %q is in use by Docker Container: %q", device.PathOnHost, containerJSON.ID)
					ret.insert(podContainer.uid, containerIdentifier.name, device.PathOnHost)
				}
			}
		}
	}
	return ret
}

func isValidPath(path string) bool {
	return regexp.MustCompile(solarflareFullpathRE).MatchString(path)
}
