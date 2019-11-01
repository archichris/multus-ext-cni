// Copyright 2015 CNI authors
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

package allocator

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/types"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/intel/multus-cni/logging"
)

// The top-level network config - IPAM plugins are passed the full configuration
// of the calling plugin, not just the IPAM section.

var (
	fixSuffix        = "fix"
	defaultApplyUnit = uint32(4)
)

type Net struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Master     string      `json:"master"`
	CNIVersion string      `json:"cniVersion"`
	IPAM       *IPAMConfig `json:"ipam"`
	// Vxlan         *VxlanNetConf `json:"vxlan"`
	RuntimeConfig struct { // The capability arg
		IPRanges []RangeSet `json:"ipRanges,omitempty"`
	} `json:"runtimeConfig,omitempty"`
	Args *struct {
		A *IPAMArgs `json:"cni"`
	} `json:"args"`
	LogFile  string `json:"logFile"`
	LogLevel string `json:"logLevel"`
}

// type VxlanNetConf struct {
// 	VxlanId  int  `json:"vxlanId"`
// 	Port     int  `json:"port"`
// 	Learning bool `json:"learning"`
// 	GBP      bool `json:"gbp"`
// }

// IPAMConfig represents the IP related network configuration.
// This nests Range because we initially only supported a single
// range directly, and wish to preserve backwards compatability
type IPAMConfig struct {
	*Range
	Name       string
	Type       string         `json:"type"`
	Routes     []*types.Route `json:"routes"`
	DataDir    string         `json:"dataDir"`
	ResolvConf string         `json:"resolvConf"`
	Ranges     []RangeSet     `json:"ranges"`
	FixRange   *Range         `json:"fixRange"`
	IPArgs     []net.IP       `json:"-"` // Requested IPs from CNI_ARGS and args
	ApplyUnit  uint32         `json:"applyUnit,omitempty"`
	AllocGW    bool           `json:"allocGW,omitempty"`
	PodName    string
	K8sNs      string
	IsFixIP    bool
	// FixNets    []string
}

type IPAMEnvArgs struct {
	types.CommonArgs
	IP                net.IP                     `json:"ip,omitempty"`
	K8S_POD_NAMESPACE types.UnmarshallableString `json:"k8sPodNamespace,omitempty"`
	K8S_POD_NAME      types.UnmarshallableString `json:"k8sPodName,omitempty"`
	Fix               types.UnmarshallableString `json:"extEnv,omitempty"`
}

type IPAMArgs struct {
	IPs []net.IP `json:"ips"`
}

type RangeSet []Range

type Range struct {
	SimpleRange
	// RangeStart net.IP      `json:"rangeStart,omitempty"` // The first ip, inclusive
	// RangeEnd   net.IP      `json:"rangeEnd,omitempty"`   // The last ip, inclusive
	Subnet   types.IPNet `json:"subnet"`
	Gateway  net.IP      `json:"gateway,omitempty"`
	Reserves []net.IP    `json:"reserves,omitempty"`
}

type SimpleRange struct {
	RangeStart net.IP `json:"rangeStart,omitempty"` // The first ip, inclusive
	RangeEnd   net.IP `json:"rangeEnd,omitempty"`   // The last ip, inclusive
}

// NewIPAMConfig creates a NetworkConfig from the given network name.
func LoadIPAMConfig(bytes []byte, envArgs string) (*Net, string, error) {
	n := Net{}
	if err := json.Unmarshal(bytes, &n); err != nil {
		return nil, "", err
	}

	if n.IPAM == nil {
		return nil, "", fmt.Errorf("IPAM config missing 'ipam' key")
	}

	// Logging
	if n.LogFile != "" {
		logging.SetLogFile(n.LogFile)
	}
	if n.LogLevel != "" {
		logging.SetLogLevel(n.LogLevel)
	}

	// Parse custom IP from both env args *and* the top-level args config
	if envArgs != "" {
		e := IPAMEnvArgs{}
		err := types.LoadArgs(envArgs, &e)
		if err != nil {
			return nil, "", err
		}

		if e.IP != nil {
			n.IPAM.IPArgs = []net.IP{e.IP}
		}
		if e.K8S_POD_NAME != "" {
			n.IPAM.PodName = string(e.K8S_POD_NAME)
		}
		if e.K8S_POD_NAMESPACE != "" {
			n.IPAM.K8sNs = string(e.K8S_POD_NAMESPACE)
		}
		if e.Fix != "" {
			for _, t := range strings.Split(string(e.Fix), ",") {
				if strings.ToLower(t) == strings.ToLower(n.Name) {
					n.IPAM.IsFixIP = true
				}
			}
		}
	}

	if n.Args != nil && n.Args.A != nil && len(n.Args.A.IPs) != 0 {
		n.IPAM.IPArgs = append(n.IPAM.IPArgs, n.Args.A.IPs...)
	}

	for idx := range n.IPAM.IPArgs {
		if err := canonicalizeIP(&n.IPAM.IPArgs[idx]); err != nil {
			return nil, "", fmt.Errorf("cannot understand ip: %v", err)
		}
	}

	// If a single range (old-style config) is specified, prepend it to
	// the Ranges array
	if n.IPAM.Range != nil && n.IPAM.Range.Subnet.IP != nil {
		n.IPAM.Ranges = append([]RangeSet{{*n.IPAM.Range}}, n.IPAM.Ranges...)
	}
	n.IPAM.Range = nil

	// If a range is supplied as a runtime config, prepend it to the Ranges
	if len(n.RuntimeConfig.IPRanges) > 0 {
		n.IPAM.Ranges = append(n.RuntimeConfig.IPRanges, n.IPAM.Ranges...)
	}

	if len(n.IPAM.Ranges) == 0 {
		return nil, "", fmt.Errorf("no IP ranges specified")
	}

	// Validate all ranges
	numV4 := 0
	numV6 := 0
	for i := range n.IPAM.Ranges {
		if err := n.IPAM.Ranges[i].Canonicalize(); err != nil {
			return nil, "", fmt.Errorf("invalid range set %d: %s", i, err)
		}

		if n.IPAM.Ranges[i][0].RangeStart.To4() != nil {
			numV4++
		} else {
			numV6++
		}
	}

	// CNI spec 0.2.0 and below supported only one v4 and v6 address
	if numV4 > 1 || numV6 > 1 {
		for _, v := range types020.SupportedVersions {
			if n.CNIVersion == v {
				return nil, "", fmt.Errorf("CNI version %v does not support more than 1 address per family", n.CNIVersion)
			}
		}
	}

	// Check for overlaps
	l := len(n.IPAM.Ranges)
	for i, p1 := range n.IPAM.Ranges[:l-1] {
		for j, p2 := range n.IPAM.Ranges[i+1:] {
			if p1.Overlaps(&p2) {
				return nil, "", fmt.Errorf("range set %d overlaps with %d", i, (i + j + 1))
			}
		}
	}

	n.IPAM.Name = n.Name

	if n.IPAM.FixRange != nil {
		if err := n.IPAM.FixRange.Canonicalize(); err != nil {
			return nil, "", fmt.Errorf("invalid fixRange set %v, %s", n.IPAM.FixRange, err)
		}
	}

	if n.IPAM.ApplyUnit == 0 {
		n.IPAM.ApplyUnit = defaultApplyUnit
	}

	return &n, n.CNIVersion, nil
}
