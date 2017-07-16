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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

const (
	deviceSock = "device.sock"
)

// MockDevicePlugin is a mock device plugin that can be used for testing
type MockDevicePlugin struct {
	kubeletSocket string

	ndevs  int
	vendor string
	kind   string

	waitToKill time.Duration

	devs   []*pluginapi.Device
	server *grpc.Server

	deviceErrorChan chan *pluginapi.Device

	client         pluginapi.PluginRegistrationClient
	c              *grpc.ClientConn
	heartbeatTimer *time.Timer
}

// Discover is the function listing the devices
func (m *MockDevicePlugin) Discover(e *pluginapi.Empty,
	deviceStream pluginapi.DeviceManager_DiscoverServer) error {

	for _, dev := range m.devs {
		deviceStream.Send(dev)
	}

	return nil
}

// Monitor is the function monitoring the devices
func (m *MockDevicePlugin) Monitor(e *pluginapi.Empty,
	deviceStream pluginapi.DeviceManager_MonitorServer) error {

	for {
		d := <-m.deviceErrorChan
		time.Sleep(m.waitToKill)

		h := NewDeviceHealth(d.Name, d.Kind, m.vendor, pluginapi.Unhealthy)
		err := deviceStream.Send(h)

		if err != nil {
			fmt.Printf("Error while monitoring: %+v", err)
		}
	}
}

// Allocate is the function Allocating a set of devices
func (m *MockDevicePlugin) Allocate(ctx context.Context,
	r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {

	var response pluginapi.AllocateResponse

	response.Envs = append(response.Envs, &pluginapi.KeyValue{
		Key:   "TEST_ENV_VAR",
		Value: "FOO",
	})

	response.Mounts = append(response.Mounts, &pluginapi.Mount{
		Name:      "mount-abc",
		HostPath:  "/tmp",
		MountPath: "/device-plugin",
		ReadOnly:  false,
	})

	m.deviceErrorChan <- r.Devices[0]

	return &response, nil
}

// Deallocate is the function Deallocating a set of devices
func (m *MockDevicePlugin) Deallocate(ctx context.Context,
	r *pluginapi.DeallocateRequest) (*pluginapi.Empty, error) {

	return &pluginapi.Empty{}, nil
}

// Stop is the function stoping the gRPC server and the heartbeat
func (m *MockDevicePlugin) Stop() {
	// Dial Registry wasn't called
	if m.heartbeatTimer != nil {
		m.heartbeatTimer.Stop()
	}

	if m.c != nil {
		m.c.Close()
	}

	m.server.Stop()
}

// NewMockDevicePlugin creates a mock device plugin
func NewMockDevicePlugin(kubeletSocket, vendor, kind string, ndevs int,
	waitToKill time.Duration) *MockDevicePlugin {

	plugin := &MockDevicePlugin{
		kubeletSocket: kubeletSocket,

		vendor:     vendor,
		kind:       kind,
		ndevs:      ndevs,
		waitToKill: waitToKill,

		deviceErrorChan: make(chan *pluginapi.Device),
	}

	for i := 0; i < plugin.ndevs; i++ {
		plugin.devs = append(plugin.devs, NewDevice(strconv.Itoa(i),
			plugin.kind, plugin.vendor))
	}

	return plugin
}

// StartMockDevicePluginServer creates and start a MockDevicePlugin
func StartMockDevicePluginServer(kubeletSocket, vendor, kind string, ndevs int,
	waitToKill time.Duration) (*MockDevicePlugin, error) {

	dir, _ := filepath.Split(kubeletSocket)
	socketPath := dir + deviceSock

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sock, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	plugin := NewMockDevicePlugin(kubeletSocket, vendor, kind, ndevs, waitToKill)
	plugin.server = grpc.NewServer([]grpc.ServerOption{}...)

	pluginapi.RegisterDeviceManagerServer(plugin.server, plugin)
	go plugin.server.Serve(sock)

	return plugin, nil
}

// DialRegistry dials the kubelet registry
func (m *MockDevicePlugin) DialRegistry() error {
	var err error

	m.c, err = grpc.Dial(m.kubeletSocket, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return err
	}

	m.client = pluginapi.NewPluginRegistrationClient(m.c)
	resp, err := m.client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: deviceSock,
		Vendor:     m.vendor,
	})

	if err != nil {
		return err
	}

	if resp.Error != "" {
		return fmt.Errorf("%s", resp.Error)
	}

	m.heartbeatTimer = time.NewTimer(5 * time.Second)
	go func() {
		for range m.heartbeatTimer.C {
			r, err := m.client.Heartbeat(context.Background(),
				&pluginapi.HeartbeatRequest{Vendor: m.vendor})

			if err != nil {
				m.heartbeatTimer.Stop()
				return
			}

			if r.Response == pluginapi.HeartbeatError {
				m.heartbeatTimer.Stop()
				return
			}
		}
	}()

	return nil
}
