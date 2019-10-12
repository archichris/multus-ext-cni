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
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/intel/multus-cni/logging"
	"github.com/vishvananda/netlink"
)

type NetConf struct {
	types.NetConf
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
	logging.SetLogFile("/tmp/multus-vxlan.log")
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
	return n, n.CNIVersion, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	logging.Debugf("%v", args.StdinData)
	n, cniVersion, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	br, result, err := bridgeAdd(args, n)
	if err != nil {
		return logging.Errorf("bridgeAdd failed, %v", err)
	}

	vxlan, err := setupVxlan(&(n.Vxlan))
	if err != nil {
		return logging.Errorf("setupVxlan failed, %v", err)
	}

	if vxlan.Attrs().MasterIndex != br.Attrs().Index {
		err = netlink.LinkSetMaster(vxlan, br)
		if err != nil {
			return logging.Errorf("LinkSetMaster failed, %v", err)
		}
	}
	result.CNIVersion = cniVersion

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
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("multus-vxlan"))
}