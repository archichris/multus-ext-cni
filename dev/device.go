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

package dev

import (
	"fmt"
	"net"
	"syscall"

	"github.com/intel/multus-cni/logging"
	"github.com/vishvananda/netlink"
)

func AddFDB(index int, mac net.HardwareAddr, ip net.IP) error {
	logging.Debugf("calling NeighAppend to %v: %v, %v", index, mac, ip)
	return netlink.NeighAppend(&netlink.Neigh{
		LinkIndex:    index,
		State:        netlink.NUD_PERMANENT,
		Family:       syscall.AF_BRIDGE,
		Flags:        netlink.NTF_SELF,
		IP:           ip,
		HardwareAddr: mac,
	})
}

func DelFDB(index int, mac net.HardwareAddr, ip net.IP) error {
	logging.Debugf("calling DelFDB to %v: %v, %v", index, mac, ip)
	return netlink.NeighDel(&netlink.Neigh{
		LinkIndex:    index,
		Family:       syscall.AF_BRIDGE,
		Flags:        netlink.NTF_SELF,
		IP:           ip,
		HardwareAddr: mac,
	})
}

func AddARP(index int, mac net.HardwareAddr, ip net.IP) error {
	logging.Debugf("calling AddARP to %v: %v, %v", index, mac, ip)
	return netlink.NeighSet(&netlink.Neigh{
		LinkIndex:    index,
		State:        netlink.NUD_PERMANENT,
		Type:         syscall.RTN_UNICAST,
		IP:           ip,
		HardwareAddr: mac,
	})
}

func DelARP(index int, mac net.HardwareAddr, ip net.IP) error {
	logging.Debugf("calling DelARP to %v: %v, %v", index, mac, ip)
	return netlink.NeighDel(&netlink.Neigh{
		LinkIndex:    index,
		State:        netlink.NUD_PERMANENT,
		Type:         syscall.RTN_UNICAST,
		IP:           ip,
		HardwareAddr: mac,
	})
}

func GetIfaceAddrs(iface *net.Interface) ([]netlink.Addr, error) {
	link := &(netlink.Device{
		netlink.LinkAttrs{
			Index: iface.Index,
		},
	})

	return netlink.AddrList(link, syscall.AF_INET)
}

func GetIfaceIP4Addr(iface *net.Interface) (net.IP, error) {
	addrs, err := GetIfaceAddrs(iface)
	if err != nil {
		return nil, err
	}

	// prefer non link-local addr
	var ll net.IP

	for _, addr := range addrs {
		if addr.IP.To4() == nil {
			continue
		}

		if addr.IP.IsGlobalUnicast() {
			return addr.IP, nil
		}

		if addr.IP.IsLinkLocalUnicast() {
			ll = addr.IP
		}
	}

	if ll != nil {
		// didn't find global but found link-local. it'll do.
		return ll, nil
	}

	return nil, fmt.Errorf("No IPv4 address found for given interface")
}
