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

package nic

import "k8s.io/kubernetes/pkg/api/v1"

// NICManager manages NICs on a local node.
// Implementations are expected to be thread safe.
type NICManager interface {
	// Start logically initializes NICManager
	Start() error
	// Capacity returns the total number of NICs on the node.
	Capacity() v1.ResourceList
	// AllocateNIC attempts to allocate NICs for input container.
	// Returns paths to allocated NICs and nil on success.
	// Returns an error on failure.
	AllocateNIC(*v1.Pod, *v1.Container) ([]string, error)
}
