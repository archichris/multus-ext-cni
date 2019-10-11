package main

import (
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

type VxlanNetConf struct {
	IfName   string `json:"ifName"`
	VxlanId  int    `json:"vxlanId"`
	Port     int    `json:"port"`
	Learning bool   `json:"learning"`
	GBP      bool   `json:"gbp"`
}

func ensureLink(vxlan *netlink.Vxlan) (*netlink.Vxlan, error) {
	err := netlink.LinkAdd(vxlan)
	if err == syscall.EEXIST {
		// it's ok if the device already exists as long as config is similar
		// log.V(1).Infof("VXLAN device already exists")
		existing, err := netlink.LinkByName(vxlan.Name)
		if err != nil {
			return nil, err
		}

		incompat := vxlanLinksIncompat(vxlan, existing)
		if incompat == "" {
			// log.V(1).Infof("Returning existing device")
			return existing.(*netlink.Vxlan), nil
		}

		// delete existing
		// log.Warningf("%q already exists with incompatable configuration: %v; recreating device", vxlan.Name, incompat)
		if err = netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("failed to delete interface: %v", err)
		}

		// create new
		if err = netlink.LinkAdd(vxlan); err != nil {
			return nil, fmt.Errorf("failed to create vxlan interface: %v", err)
		}
	} else if err != nil {
		return nil, err
	}

	ifindex := vxlan.Index
	link, err := netlink.LinkByIndex(vxlan.Index)
	if err != nil {
		return nil, fmt.Errorf("can't locate created vxlan device with index %v", ifindex)
	}

	var ok bool
	if vxlan, ok = link.(*netlink.Vxlan); !ok {
		return nil, fmt.Errorf("created vxlan device with index %v is not vxlan", ifindex)
	}

	return vxlan, nil
}

func vxlanLinksIncompat(l1, l2 netlink.Link) string {
	if l1.Type() != l2.Type() {
		return fmt.Sprintf("link type: %v vs %v", l1.Type(), l2.Type())
	}

	v1 := l1.(*netlink.Vxlan)
	v2 := l2.(*netlink.Vxlan)

	if v1.VxlanId != v2.VxlanId {
		return fmt.Sprintf("vni: %v vs %v", v1.VxlanId, v2.VxlanId)
	}

	if v1.VtepDevIndex > 0 && v2.VtepDevIndex > 0 && v1.VtepDevIndex != v2.VtepDevIndex {
		return fmt.Sprintf("vtep (external) interface: %v vs %v", v1.VtepDevIndex, v2.VtepDevIndex)
	}

	if len(v1.SrcAddr) > 0 && len(v2.SrcAddr) > 0 && !v1.SrcAddr.Equal(v2.SrcAddr) {
		return fmt.Sprintf("vtep (external) IP: %v vs %v", v1.SrcAddr, v2.SrcAddr)
	}

	if len(v1.Group) > 0 && len(v2.Group) > 0 && !v1.Group.Equal(v2.Group) {
		return fmt.Sprintf("group address: %v vs %v", v1.Group, v2.Group)
	}

	if v1.L2miss != v2.L2miss {
		return fmt.Sprintf("l2miss: %v vs %v", v1.L2miss, v2.L2miss)
	}

	if v1.Port > 0 && v2.Port > 0 && v1.Port != v2.Port {
		return fmt.Sprintf("port: %v vs %v", v1.Port, v2.Port)
	}

	if v1.GBP != v2.GBP {
		return fmt.Sprintf("gbp: %v vs %v", v1.GBP, v2.GBP)
	}

	return ""
}

func setupVxlan(cfg *VxlanNetConf) (*netlink.Vxlan, error) {

	iface, err := net.InterfaceByName(cfg.IfName)
	if err != nil {
		return nil, fmt.Errorf("error looking up interface %s: %s", cfg.IfName, err)
	}

	ifaceAddr, err := getIfaceIP4Addr(iface)
	if err != nil {
		return nil, fmt.Errorf("failed to find IPv4 address for interface %s", iface.Name)
	}

	if iface.MTU == 0 {
		return nil, fmt.Errorf("failed to determine MTU for %s interface", ifaceAddr)
	}
	linkCfg := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("multus.%v.%v", cfg.IfName, cfg.VxlanId),
		},
		VxlanId:      cfg.VxlanId,
		VtepDevIndex: iface.Index,
		SrcAddr:      ifaceAddr,
		Port:         cfg.Port,
		Learning:     cfg.Learning,
		GBP:          cfg.GBP,
	}

	vxlan, err := ensureLink(linkCfg)
	if err != nil {
		return nil, err
	}
	return vxlan, err
}

func getIfaceAddrs(iface *net.Interface) ([]netlink.Addr, error) {
	link := &(netlink.Device{
		netlink.LinkAttrs{
			Index: iface.Index,
		},
	})

	return netlink.AddrList(link, syscall.AF_INET)
}

func getIfaceIP4Addr(iface *net.Interface) (net.IP, error) {
	addrs, err := getIfaceAddrs(iface)
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
