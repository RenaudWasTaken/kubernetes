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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

const (
	deviceKind    = "device"
	waitToKill    = 1
	kubeletSocket = "/tmp/kubelet.sock"
	deviceVendor  = "fooVendor"
)

var (
	nDevices = 3
)

// TestManagerDiscovery tests that device plugin's Discovery method
// is called when registering
func TestManagerDiscovery(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	devs, ok := mgr.Devices()[deviceKind]
	require.True(t, ok)

	require.Len(t, devs, nDevices)
	for _, d := range devs {
		_, ok = HasDevice(d, plugin.devs)
		require.True(t, ok)
	}
}

// TestManagerAllocation tests that device plugin's Allocation and Deallocation method
// allocates correctly the devices
// This also tests the RM of the manager
func TestManagerAllocation(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	for i := 1; i < nDevices; i++ {
		devs, resp, err := mgr.Allocate("device", i)
		require.NoError(t, err)

		require.Len(t, devs, i)
		require.Len(t, resp[0].Envs, 1)
		require.Len(t, resp[0].Mounts, 1)

		require.Len(t, mgr.Available()["device"], nDevices-i)

		// Deallocation test
		mgr.Deallocate(devs)
		time.Sleep(time.Millisecond * 500)
		require.Len(t, mgr.Available()["device"], nDevices)
	}
}

// TestManagerAllocation tests that device plugin's Allocation and Deallocation method
func TestManagerMonitoring(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	devs, _, err := mgr.Allocate("device", 1)
	require.NoError(t, err)

	// Monitoring test
	time.Sleep(waitToKill*time.Second + 500*time.Millisecond)
	unhealthyDev := devs[0]

	devs = mgr.Devices()[deviceKind]
	i, ok := HasDevice(unhealthyDev, devs)

	require.True(t, ok)
	require.Equal(t, pluginapi.Unhealthy, devs[i].Health)
}

func TestManagerDevicePluginRestart(t *testing.T) {
	mgr, err := NewManager(kubeletSocket, nil, nil, monitorCallback)
	require.NoError(t, err)

	plugin, err := StartMockDevicePluginServer(kubeletSocket, deviceVendor, deviceKind,
		nDevices, time.Millisecond*500)
	require.NoError(t, err)

	err = plugin.DialRegistry()
	require.NoError(t, err)

	// Simulate crash by stoping and restarting the device plugin
	plugin.Stop()

	plugin, err = StartMockDevicePluginServer(kubeletSocket, deviceVendor, deviceKind,
		nDevices, time.Millisecond*500)
	require.NoError(t, err)

	err = plugin.DialRegistry()
	require.NoError(t, err)

	// Check that the devices have not been registered twice
	devs, ok := mgr.Devices()[deviceKind]
	require.True(t, ok)

	require.Len(t, devs, nDevices)
	for _, d := range devs {
		_, ok = HasDevice(d, plugin.devs)
		require.True(t, ok)
	}

	plugin.Stop()
	mgr.Stop()
}

// Setup gives you the garantee that if no error has been returned
// the device plugin registered itself against kubelet and kubelet
// discovered the devices
// Note that this is tested by the TestManagerDiscovery
func setup() (*Manager, *MockDevicePlugin, error) {
	mgr, err := NewManager(kubeletSocket, nil, nil, monitorCallback)
	if err != nil {
		return nil, nil, err
	}

	plugin, err := StartMockDevicePluginServer(kubeletSocket, deviceVendor, deviceKind,
		nDevices, time.Millisecond*500)
	if err != nil {
		return nil, nil, err
	}

	err = plugin.DialRegistry()
	if err != nil {
		return nil, nil, err
	}

	return mgr, plugin, nil
}

func teardown(mgr *Manager, plugin *MockDevicePlugin) {
	plugin.Stop()
	mgr.Stop()
}

func monitorCallback(d *pluginapi.Device) {
}
