// Copyright 2014 CNI authors
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
	"encoding/json"
	"os"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-vxlan/backend/etcdv3cli"
	"github.com/vishvananda/netlink"
)

type NetConf struct {
	types.NetConf
	Master string `json:"master"`
	BridgeNetConf
	Vxlan    VxlanNetConf `json:"vxlan"`
	LogFile  string       `json:"logFile"`
	LogLevel string       `json:"logLevel"`
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	runtime.LockOSThread()
	//for debug
	logging.SetLogFile("/var/log/multus-vxlan.log")
	logging.SetLogLevel("debug")
}

func loadNetConf(bytes []byte) (*NetConf, string, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", logging.Errorf("Unmarshal failed, %v", err)
	}
	// Logging
	if n.LogFile != "" {
		logging.SetLogFile(n.LogFile)
	}
	if n.LogLevel != "" {
		logging.SetLogLevel(n.LogLevel)
	}
	n.Vxlan.Master = n.Master
	if n.BrName == "" {
		n.BrName = n.Master
	}
	return n, n.CNIVersion, nil
}

func cmdAdd(args *skel.CmdArgs) error {

	logging.Debugf(os.Getenv("CNI_ARGS"))
	n, cniVersion, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}
	logging.Debugf("%v", n)

	vxlan, err := setupVxlan(&(n.Vxlan))
	if err != nil {
		return logging.Errorf("setupVxlan failed, %v", err)
	}

	// var result *current.Result
	br, result, err := bridgeAdd(args, n)
	if err != nil {
		return logging.Errorf("bridgeAdd failed, %v", err)
	}

	if vxlan.Attrs().MasterIndex != br.Attrs().Index {
		err = netlink.LinkSetMaster(vxlan, br)
		if err != nil {
			return logging.Errorf("LinkSetMaster failed, %v", err)
		}
	}

	if err := netlink.LinkSetUp(vxlan); err != nil {
		return logging.Errorf("Enable vxlan failed")
	}

	// if n.IPAM.Type != "" {
	// 	for _, ip := range result.IPs {
	// 		i := ip.Address
	// 		i.IP = i.IP.Mask(i.Mask)
	// 		err := netlink.RouteAdd(&netlink.Route{LinkIndex: vxlan.Attrs().Index, Scope: netlink.SCOPE_UNIVERSE, Dst: &i})
	// 		if err != nil {
	// 			logging.Errorf("RouteAdd %v Dst:%v, failed, %v", vxlan.Attrs().Index, i, err)
	// 		} else {
	// 			logging.Verbosef("RouteAdd %v Dst:%v successed", vxlan.Attrs().Index, i)
	// 		}
	// 	}
	// }

	etcdv3cli.RecVxlan(n.Name, vxlan)

	result.CNIVersion = cniVersion

	//Todo write neighbor to etcd /vxlan/<netname>/arp/<mainip>-<id>/<mac>:<ip>
	return types.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	n, _, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	return bridgeDel(args, n)
}

func cmdCheck(args *skel.CmdArgs) error {
	n, _, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}
	return bridgeCheck(args, n)
}

func main() {
	// skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("multus-vxlan"))
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.PluginSupports("0.3.0", "0.3.1", "0.4.0"), bv.BuildString("multus-vxlan"))

}
