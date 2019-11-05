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

package vxlan

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
)

type NetConf struct {
	types.NetConf
	Master string `json:"master"`
	BridgeNetConf
	Vxlan    VxlanNetConf `json:"vxlan"`
	LogFile  string       `json:"logFile"`
	LogLevel string       `json:"logLevel"`
}

type BridgeNetConf struct {
	// BrName       string `json:"bridge"`
	IsGW         bool   `json:"isGateway"`
	IsDefaultGW  bool   `json:"isDefaultGateway"`
	ForceAddress bool   `json:"forceAddress"`
	IPMasq       bool   `json:"ipMasq"`
	MTU          int    `json:"mtu"`
	HairpinMode  bool   `json:"hairpinMode"`
	PromiscMode  bool   `json:"promiscMode"`
	Vlan         int    `json:"vlan"`
	BrName       string `json:"brName"`
}

type gwInfo struct {
	gws               []net.IPNet
	family            int
	defaultRouteFound bool
}

type VxlanNetConf struct {
	Master   string
	VxlanId  int  `json:"vxlanId"`
	Port     int  `json:"port"`
	Learning bool `json:"learning"`
	GBP      bool `json:"gbp"`
}
