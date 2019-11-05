package vxlan

import (
	"fmt"
	"net"
	"syscall"

	"github.com/intel/multus-cni/dev"
	"github.com/intel/multus-cni/logging"
	"github.com/vishvananda/netlink"
)

func ensureLink(vxlan *netlink.Vxlan) (*netlink.Vxlan, error) {
	err := netlink.LinkAdd(vxlan)
	if err == syscall.EEXIST {
		// it's ok if the device already exists as long as config is similar
		// log.V(1).Infof("VXLAN device already exists")
		existing, err := netlink.LinkByName(vxlan.Name)
		if err != nil {
			return nil, logging.Errorf("try to get vxlan %v failed, %v", vxlan.Name, err)
		}

		incompat := vxlanLinksIncompat(vxlan, existing)
		if incompat == "" {
			// log.V(1).Infof("Returning existing device")
			return existing.(*netlink.Vxlan), nil
		}

		// delete existing
		// log.Warningf("%q already exists with incompatable configuration: %v; recreating device", vxlan.Name, incompat)
		if err = netlink.LinkDel(existing); err != nil {
			return nil, logging.Errorf("failed to delete interface: %v", err)
		}

		// create new
		if err = netlink.LinkAdd(vxlan); err != nil {
			return nil, logging.Errorf("failed to create vxlan interface: %v", err)
		}
	} else if err != nil {
		return nil, logging.Errorf("LinkAdd %v failed, %v", vxlan.Name, err)
	}

	ifindex := vxlan.Index
	link, err := netlink.LinkByIndex(vxlan.Index)
	if err != nil {
		return nil, logging.Errorf("can't locate created vxlan device with index %v", ifindex)
	}

	var ok bool
	if vxlan, ok = link.(*netlink.Vxlan); !ok {
		return nil, logging.Errorf("created vxlan device with index %v is not vxlan", ifindex)
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

func SetupVxlan(cfg *VxlanNetConf) (*netlink.Vxlan, net.IP, error) {

	iface, err := net.InterfaceByName(cfg.Master)
	if err != nil {
		return nil, nil, logging.Errorf("error looking up interface %s: %s", cfg.Master, err)
	}

	ifaceAddr, err := dev.GetIfaceIP4Addr(iface)
	if err != nil {
		return nil, nil, logging.Errorf("failed to find IPv4 address for interface %s", iface.Name)
	}

	if iface.MTU == 0 {
		return nil, nil, logging.Errorf("failed to determine MTU for %s interface", ifaceAddr)
	}
	linkCfg := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("mulvx.%v", cfg.VxlanId),
		},
		VxlanId:      cfg.VxlanId,
		VtepDevIndex: iface.Index,
		SrcAddr:      ifaceAddr,
		Port:         cfg.Port,
		Learning:     cfg.Learning,
		GBP:          cfg.GBP,
		L2miss:       true,
		L3miss:       true,
	}

	vxlan, err := ensureLink(linkCfg)
	if err != nil {
		return nil, nil, err
	}
	return vxlan, ifaceAddr, err
}
