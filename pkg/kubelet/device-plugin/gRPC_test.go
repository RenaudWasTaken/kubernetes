/*
Copyright 2016 The Kubernetes Authors.

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

package deviceplugin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

func TestInvalidHandleDeviceUnhealthy(t *testing.T) {
	mgr, err := NewManager(kubeletSocket, nil, nil, monitorCallback)
	require.NoError(t, err)

	err = mgr.registry.handleDeviceUnhealthy(&pluginapi.Device{
		Name:   "foo",
		Kind:   "bar",
		Vendor: "A Different Vendor",
	}, deviceVendor)
	require.True(t, strings.HasPrefix(err.Error(), pluginapi.ErrVendorMismatch))

	d := &pluginapi.Device{
		Name:   "foo",
		Kind:   "bar",
		Vendor: deviceVendor,
	}

	err = mgr.registry.handleDeviceUnhealthy(d, deviceVendor)
	require.True(t, strings.HasPrefix(err.Error(), ErrDevicePluginUnknown))

	mgr.devices[d.Kind] = nil
	mgr.available[d.Kind] = nil

	err = mgr.registry.handleDeviceUnhealthy(d, deviceVendor)
	require.True(t, strings.HasPrefix(err.Error(), ErrDeviceUnknown))

	mgr.Stop()

}
