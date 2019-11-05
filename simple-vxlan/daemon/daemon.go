// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/intel/multus-cni/dev"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/simple-vxlan/vxlan"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/net/context"
)

var (
	_, GuestNet, _ = net.ParseCIDR("192.168.0.0/16")
	_, HostNet, _  = net.ParseCIDR("192.168.0.0/16")
	ifaceAddr      = net.IPv4zero
)

func init() {
	//for debug
	logging.SetLogFile("/var/log/simple-daemon.log")
	logging.SetLogLevel("debug")
}

func main() {
	// install signal process
	logging.Verbosef("install signals")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		shutdownHandler(ctx, sigs, cancel)
		logging.Verbosef("shutdownHandler terminated")
		wg.Done()
	}()

	iface, err := net.InterfaceByName("eth0")
	if err != nil {
		logging.Errorf("error looking up interface %s: %s", "eth0", err)
		return
	}

	ifaceAddr, err = dev.GetIfaceIP4Addr(iface)
	if err != nil {
		logging.Errorf("failed to find IPv4 address for interface %s", iface.Name)
		return
	}

	wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		MonitorMisses()
		wg.Done()
	}()

	logging.Verbosef("Waiting for all goroutines to exit")
	// Block waiting for all the goroutines to finish.
	wg.Wait()
	logging.Verbosef("Exiting cleanly...")
	os.Exit(0)
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	// Wait for the context do be Done or for the signal to come in to shutdown.
	select {
	case <-ctx.Done():
		logging.Verbosef("Stopping shutdownHandler...")
	case <-sigs:
		// Call cancel on the context to close everything down.
		cancel()
		logging.Verbosef("shutdownHandler sent cancel signal...")
	}

	// Unregister to get default OS nuke behaviour in case we don't exit cleanly
	signal.Stop(sigs)
}

func handleMiss(miss *netlink.Neigh) {
	switch {
	case len(miss.IP) == 0 && len(miss.HardwareAddr) == 0:
		logging.Debugf("Ignoring nil miss")

	case len(miss.HardwareAddr) == 0:
		handleL3Miss(miss)

	case len(miss.IP) == 0:
		handleL2Miss(miss)

	default:
		logging.Verbosef("Ignoring not a miss: %v, %v", miss.HardwareAddr, miss.IP)
	}
}

func handleL2Miss(miss *netlink.Neigh) {
	logging.Debugf("l2 miss %+v", *miss)
	// netIP := miss.IP.Mask(GuestNet.Mask)
	// logging.Debugf("miss ip net is %v", netIP)
	// if ip.Cmp(netIP, GuestNet.IP) != 0 {
	// 	logging.Debugf("%v not match %v", netIP, miss.IP)
	// }
	host, guest := vxlan.MacToIP(miss.HardwareAddr)
	l, err := netlink.LinkByName("mulvx.201")
	if err != nil {
		//It may be nomral when no container run in this node
		logging.Verbosef("get interface %v failed, %v", "mulvx.201", err)
		return
	}

	vx, ok := l.(*netlink.Vxlan)
	if !ok {
		logging.Errorf("%s already exists but is not a vxlan", "mulvx.201")
		return
	}

	logging.Debugf("l2miss get %v:%v, local:%v", host, guest, vx.SrcAddr)
	if ip.Cmp(host, vx.SrcAddr) == 0 {
		return
	}

	err = dev.AddFDB(vx.Index, vx.HardwareAddr, host)
	if err != nil {
		logging.Errorf("Add fdb %v, %v, %v failed, %v", vx.Index, vx.HardwareAddr, host, err)
		return
	}
}

func handleL3Miss(miss *netlink.Neigh) {
	logging.Debugf("l3 miss %+v", *miss)
}

func MonitorMisses() {
	// nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_NEIGH, syscall.RTM_DELNEIGH, syscall.RTM_GETNEIGH, syscall.RTM_NEWNEIGH, syscall.RTNLGRP_NOTIFY, syscall.RTNLGRP_LINK)
	// nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_LINK, syscall.RTNLGRP_NOTIFY, syscall.RTNLGRP_NEIGH)
	nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_NEIGH)
	// nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, 0xff)
	if err != nil {
		logging.Errorf("Failed to subscribe to netlink RTNLGRP_NEIGH messages")
		return
	}

	for {
		msgs, err := nlsock.Receive()
		if err != nil {
			logging.Errorf("Failed to receive from netlink: %v ", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, msg := range msgs {
			logging.Debugf("type:%v, expect:%v", msg.Header.Type, syscall.RTM_GETNEIGH)
			if msg.Header.Type != syscall.RTM_GETNEIGH {
				continue
			}
			neigh, _ := netlink.NeighDeserialize(msg.Data)
			logging.Debugf("data:%+v", msg.Header.Type, neigh)
			// if neigh.Type != syscall.RTM_NEWNEIGH {
			// 	continue
			// }
			// if neigh.IP != nil && neigh.HardwareAddr != nil {
			// 	continue
			// }
			handleMiss(neigh)
		}
	}
}
