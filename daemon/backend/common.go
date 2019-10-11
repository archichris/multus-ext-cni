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

package backend

import (
	"net"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/subnet"
)               

type ExternalInterface struct {
	Iface     *net.Interface
	IfaceAddr net.IP
	ExtAddr   net.IP
}

// Besides the entry points in the Backend interface, the backend's New()
// function receives static network interface information (like internal and
// external IP addresses, MTU, etc) which it should cache for later use if
// needed.
type Backend interface {
	// Called when the backend should create or begin managing a new network
	RegisterNetwork(ctx context.Context, wg sync.WaitGroup, config *subnet.Config) (Network, error)
}

type Network interface {
	Lease() *subnet.Lease
	MTU() int
	Run(ctx context.Context)
}

type BackendCtor func(sm subnet.Manager, ei *ExternalInterface) (Backend, error)


func LookupExtIface(ifname string, ifregex string) (*backend.ExternalInterface, error) {
	var iface *net.Interface
	var ifaceAddr net.IP
	var err error

	if len(ifname) > 0 {
		if ifaceAddr = net.ParseIP(ifname); ifaceAddr != nil {
			log.Infof("Searching for interface using %s", ifaceAddr)
			iface, err = ip.GetInterfaceByIP(ifaceAddr)
			if err != nil {
				return nil, fmt.Errorf("error looking up interface %s: %s", ifname, err)
			}
		} else {
			iface, err = net.InterfaceByName(ifname)
			if err != nil {
				return nil, fmt.Errorf("error looking up interface %s: %s", ifname, err)
			}
		}
	} else if len(ifregex) > 0 {
		// Use the regex if specified and the iface option for matching a specific ip or name is not used
		ifaces, err := net.Interfaces()
		if err != nil {
			return nil, fmt.Errorf("error listing all interfaces: %s", err)
		}

		// Check IP
		for _, ifaceToMatch := range ifaces {
			ifaceIP, err := ip.GetIfaceIP4Addr(&ifaceToMatch)
			if err != nil {
				// Skip if there is no IPv4 address
				continue
			}

			matched, err := regexp.MatchString(ifregex, ifaceIP.String())
			if err != nil {
				return nil, fmt.Errorf("regex error matching pattern %s to %s", ifregex, ifaceIP.String())
			}

			if matched {
				ifaceAddr = ifaceIP
				iface = &ifaceToMatch
				break
			}
		}

		// Check Name
		if iface == nil && ifaceAddr == nil {
			for _, ifaceToMatch := range ifaces {
				matched, err := regexp.MatchString(ifregex, ifaceToMatch.Name)
				if err != nil {
					return nil, fmt.Errorf("regex error matching pattern %s to %s", ifregex, ifaceToMatch.Name)
				}

				if matched {
					iface = &ifaceToMatch
					break
				}
			}
		}

		// Check that nothing was matched
		if iface == nil {
			var availableFaces []string
			for _, f := range ifaces {
				ip, _ := ip.GetIfaceIP4Addr(&f) // We can safely ignore errors. We just won't log any ip
				availableFaces = append(availableFaces, fmt.Sprintf("%s:%s", f.Name, ip))
			}

			return nil, fmt.Errorf("Could not match pattern %s to any of the available network interfaces (%s)", ifregex, strings.Join(availableFaces, ", "))
		}
	} else {
		log.Info("Determining IP address of default interface")
		if iface, err = ip.GetDefaultGatewayIface(); err != nil {
			return nil, fmt.Errorf("failed to get default interface: %s", err)
		}
	}

	if ifaceAddr == nil {
		ifaceAddr, err = ip.GetIfaceIP4Addr(iface)
		if err != nil {
			return nil, fmt.Errorf("failed to find IPv4 address for interface %s", iface.Name)
		}
	}

	log.Infof("Using interface with name %s and address %s", iface.Name, ifaceAddr)

	if iface.MTU == 0 {
		return nil, fmt.Errorf("failed to determine MTU for %s interface", ifaceAddr)
	}

	var extAddr net.IP

	if len(opts.publicIP) > 0 {
		extAddr = net.ParseIP(opts.publicIP)
		if extAddr == nil {
			return nil, fmt.Errorf("invalid public IP address: %s", opts.publicIP)
		}
		log.Infof("Using %s as external address", extAddr)
	}

	if extAddr == nil {
		log.Infof("Defaulting external address to interface address (%s)", ifaceAddr)
		extAddr = ifaceAddr
	}

	return &backend.ExternalInterface{
		Iface:     iface,
		IfaceAddr: ifaceAddr,
		ExtAddr:   extAddr,
	}, nil
}
