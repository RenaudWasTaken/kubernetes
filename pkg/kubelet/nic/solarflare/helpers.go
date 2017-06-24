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

import "k8s.io/apimachinery/pkg/util/sets"

type containerToNIC map[string]sets.String

// podNICs represents a list of pod to NIC mappings.
type podNICs struct {
	podNICMapping map[string]containerToNIC
}

func newPodNICs() *podNICs {
	return &podNICs{
		podNICMapping: make(map[string]containerToNIC),
	}
}
func (pNIC *podNICs) pods() sets.String {
	ret := sets.NewString()
	for k := range pNIC.podNICMapping {
		ret.Insert(k)
	}
	return ret
}

func (pNIC *podNICs) insert(podUID, contName string, device string) {
	if _, exists := pNIC.podNICMapping[podUID]; !exists {
		pNIC.podNICMapping[podUID] = make(containerToNIC)
	}
	if _, exists := pNIC.podNICMapping[podUID][contName]; !exists {
		pNIC.podNICMapping[podUID][contName] = sets.NewString()
	}
	pNIC.podNICMapping[podUID][contName].Insert(device)
}

func (pNIC *podNICs) getNICs(podUID, contName string) sets.String {
	containers, exists := pNIC.podNICMapping[podUID]
	if !exists {
		return nil
	}
	devices, exists := containers[contName]
	if !exists {
		return nil
	}
	return devices
}

func (pNIC *podNICs) delete(pods []string) {
	for _, uid := range pods {
		delete(pNIC.podNICMapping, uid)
	}
}

func (pNIC *podNICs) devices() sets.String {
	ret := sets.NewString()
	for _, containerToNIC := range pNIC.podNICMapping {
		for _, deviceSet := range containerToNIC {
			ret = ret.Union(deviceSet)
		}
	}
	return ret
}
