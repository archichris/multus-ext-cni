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

	vx "github.com/intel/multus-cni/simple-vxlan/vxlan"
	"github.com/vishvananda/netlink"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	runtime.LockOSThread()
	//for debug
	logging.SetLogFile("/var/log/multus-vxlan.log")
	logging.SetLogLevel("debug")
}

func loadNetConf(bytes []byte) (*vx.NetConf, string, error) {
	n := &vx.NetConf{}
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

	vxlan, ipaddr, err := vx.SetupVxlan(&(n.Vxlan))
	if err != nil {
		return logging.Errorf("SetupVxlan failed, %v", err)
	}

	// var result *current.Result
	br, result, err := vx.BridgeAdd(args, n, ipaddr)
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

	result.CNIVersion = cniVersion

	//Todo write neighbor to etcd /vxlan/<netname>/arp/<mainip>-<id>/<mac>:<ip>
	return types.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	n, _, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	return vx.BridgeDel(args, n)
}

func cmdCheck(args *skel.CmdArgs) error {
	n, _, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}
	return vx.BridgeCheck(args, n)
}

func main() {
	// skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("multus-vxlan"))
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.PluginSupports("0.3.0", "0.3.1", "0.4.0"), bv.BuildString("multus-vxlan"))

}
