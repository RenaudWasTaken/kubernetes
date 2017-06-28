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
	"log"
	"net"
	"os"
	//"strconv"
	"testing"
	"time"
        "os/exec"
        "bytes"
        //"syscall"
        "fmt"
        //"strconv"
        "strings"
        "io/ioutil"

  	"github.com/golang/glog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

const (
	DeviceSock1 = "device.sock.solarflare.nic"
	ServerSock1   = pluginapi.DevicePluginPath + DeviceSock1
)

type DevicePluginServer1 struct {
}

func (d *DevicePluginServer1) Init(ctx context.Context, e *pluginapi.Empty) (*pluginapi.Empty, error) {

	glog.Errorf("ramki: Init\n");

	os.Chdir("/root")

	cmd1 := exec.Command("yum", "-y", "install", "gcc", "make", "libc", "libc-devel", "perl", "autoconf", "automake", "libtool", "kernel‐devel", "binutils", "gettext", "gawk", "gcc", "sed", "make", "bash", "glibc-common", "automake", "libtool", "libpcap", "libpcap-devel", "python-devel", "glibc‐devel.i586") 
	cmdOutput1 := &bytes.Buffer{}
	cmd1.Stdout = cmdOutput1
	err1 := cmd1.Run()
	if err1 != nil {
	  os.Stderr.WriteString(err1.Error())
	}
	fmt.Print(string(cmdOutput1.Bytes()))

	return nil, nil
}

func (d *DevicePluginServer1) Stop(ctx context.Context, e *pluginapi.Empty) (*pluginapi.Empty, error) {
	return nil, nil
}

func (d *DevicePluginServer1) Discover(e *pluginapi.Empty, deviceStream pluginapi.DeviceManager_DiscoverServer) error {

	glog.Errorf("ramki: Discover\n");

        // read the whole file at once
    	b, err := ioutil.ReadFile("/proc/devices")
    	if err != nil {
        	panic(err)
    	}
    	s := string(b)

    	if (strings.Index(s, "sfc_char") > 0) {
		deviceStream.Send(&pluginapi.Device{
			Name:       "/dev/sfc_char",
			Kind:       DeviceKind,
			Vendor:     DeviceVendor,
			Properties: nil,
		})
    	}

	if (strings.Index(s, "sfc_affinity") > 0) {
		deviceStream.Send(&pluginapi.Device{
			Name:       "/dev/sfc_affnity",
			Kind:       DeviceKind,
			Vendor:     DeviceVendor,
			Properties: nil,
		})
	}

	if (strings.Index(s, "onload_epoll") > 0) {
		deviceStream.Send(&pluginapi.Device{
			Name:       "/dev/onload_epoll",
			Kind:       DeviceKind,
			Vendor:     DeviceVendor,
			Properties: nil,
		})
	}

	if (strings.Index(s, "onload_cplane") > 0) {
		deviceStream.Send(&pluginapi.Device{
			Name:       "/dev/onload_cplane",
			Kind:       DeviceKind,
			Vendor:     DeviceVendor,
			Properties: nil,
		})
	}

	// '\n' is added to avoid a match with onload_cplane and onload_epoll
	if (strings.Index(s, "onload\n") > 0) {
		deviceStream.Send(&pluginapi.Device{
			Name:       "/dev/onload",
			Kind:       DeviceKind,
			Vendor:     DeviceVendor,
			Properties: nil,
		})
	}

	return nil
}

func (d *DevicePluginServer1) Monitor(e *pluginapi.Empty, deviceStream pluginapi.DeviceManager_MonitorServer) error {
	for {
		select {
		case d := <-deviceErrorChan:
			time.Sleep(WaitToKill * time.Second)

			err := deviceStream.Send(&pluginapi.DeviceHealth{
				Name:   d.Name,
				Kind:   d.Kind,
				Vendor: DeviceVendor,
				Health: pluginapi.Unhealthy,
			})

			if err != nil {
				log.Println("error while monitoring: %+v", err)
			}
		}

		time.Sleep(time.Second)
	}
}

func (d *DevicePluginServer1) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {

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

	deviceErrorChan <- r.Devices[0]

	return &response, nil
}

func (d *DevicePluginServer1) Deallocate(ctx context.Context, r *pluginapi.DeallocateRequest) (*pluginapi.Error, error) {
	return &pluginapi.Error{}, nil
}

func StartDevicePluginServer1(t *testing.T) {
	os.Remove(ServerSock)
	sock, err := net.Listen("unix", ServerSock)
	require.NoError(t, err)

	grpcServer := grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDeviceManagerServer(grpcServer, &DevicePluginServer1{})

	go grpcServer.Serve(sock)
}

func DialRegistery1(t *testing.T) {
	c, err := grpc.Dial(pluginapi.KubeletSocket, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	require.NoError(t, err)

	client := pluginapi.NewPluginRegistrationClient(c)
	resp, err := client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: DeviceSock,
		Vendor:     DeviceVendor,
	})

	require.Len(t, resp.Error, 0)
	require.NoError(t, err)
	c.Close()
}

func monitorCallback1(d *pluginapi.Device) {
}

func TestManager1(t *testing.T) {
	mgr, err := NewManager(monitorCallback1)
	require.NoError(t, err)

	StartDevicePluginServer1(t)
	DialRegistery1(t)

	if (mgr.Devices()["device"] != nil) {

		assert.Len(t, mgr.Devices()["device"], 5)

		glog.Errorf("ramki: TestManager1 - Solarflare NIC present\n");

		devs, resp, err := mgr.Allocate("device", 1)

		require.NoError(t, err)
		assert.Len(t, resp[0].Envs, 1)
		assert.Len(t, resp[0].Mounts, 1)
		assert.Len(t, devs, 1)

		assert.Len(t, mgr.Available()["device"], 4)

		mgr.Deallocate(devs)
		assert.Len(t, mgr.Available()["device"], 5)

		time.Sleep((WaitToKill + 1) * time.Second)
		unhealthyDev := devs[0]

		devs = mgr.Devices()[DeviceKind]
		i, ok := hasDevice(unhealthyDev, devs)

		assert.True(t, ok)
		assert.Equal(t, pluginapi.Unhealthy, devs[i].Health)
	}
}
