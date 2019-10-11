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
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	log "github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/archichris/flannel/network"
	"github.com/archichris/multus/daemon/pkg/ip"
	"github.com/archichris/multus/daemon/subnet"
	"github.com/archichris/multus/daemon/subnet/etcdv2"
	"time"
	"github.com/joho/godotenv"
	"sync"

	// Backends need to be imported for their init() to get executed and them to register
	"github.com/archichris/multus/daemon/backend"
	"github.com/archichris/multus/daemon/backend/vxlan"
)


func init() {
	// can flow into journald (if running under systemd)
	flag.Set("logtostderr", "true")
}

func main() {
	
	sm, err := newSubnetManager()
	if err != nil {
		log.Error("Failed to create SubnetManager: ", err)
		os.Exit(1)
	}
	log.Infof("Created subnet manager: %s", sm.Name())

	// Register for SIGINT and SIGTERM
	log.Info("Installing signal handlers")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// This is the main context that everything should run in.
	// All spawned goroutines should exit when cancel is called on this context.
	// Go routines spawned from main.go coordinate using a WaitGroup. This provides a mechanism to allow the shutdownHandler goroutine
	// to block until all the goroutines return . If those goroutines spawn other goroutines then they are responsible for
	// blocking and returning only when cancel() is called.
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		shutdownHandler(ctx, sigs, cancel)
		wg.Done()
	}()

	NewNetManager(ctx, sm, wg).Run()

	log.Info("Waiting for all goroutines to exit")
	// Block waiting for all the goroutines to finish.
	wg.Wait()
	log.Info("Exiting cleanly...")
	os.Exit(0)
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	// Wait for the context do be Done or for the signal to come in to shutdown.
	select {
	case <-ctx.Done():
		log.Info("Stopping shutdownHandler...")
	case <-sigs:
		// Call cancel on the context to close everything down.
		cancel()
		log.Info("shutdownHandler sent cancel signal...")
	}

	// Unregister to get default OS nuke behaviour in case we don't exit cleanly
	signal.Stop(sigs)
}


func getConfig(ctx context.Context, sm subnet.Manager) (*subnet.Config, error) {
	// Retry every second until it succeeds
	for {
		config, err := sm.GetNetworkConfig(ctx)
		if err != nil {
			log.Errorf("Couldn't fetch network config: %s", err)
		} else if config == nil {
			log.Warningf("Couldn't find network config: %s", err)
		} else {
			log.Infof("Found network config - Backend type: %s", config.BackendType)
			return config, nil
		}
		select {
		case <-ctx.Done():
			return nil, errCanceled
		case <-time.After(1 * time.Second):
			fmt.Println("timed out")
		}
	}
}

func MonitorLease(ctx context.Context, sm subnet.Manager, bn backend.Network, wg *sync.WaitGroup) error {
	// Use the subnet manager to start watching leases.
	evts := make(chan subnet.Event)

	wg.Add(1)
	go func() {
		subnet.WatchLease(ctx, sm, bn.Lease().Subnet, evts)
		wg.Done()
	}()

	renewMargin := time.Duration(opts.subnetLeaseRenewMargin) * time.Minute
	dur := bn.Lease().Expiration.Sub(time.Now()) - renewMargin

	for {
		select {
		case <-time.After(dur):
			err := sm.RenewLease(ctx, bn.Lease())
			if err != nil {
				log.Error("Error renewing lease (trying again in 1 min): ", err)
				dur = time.Minute
				continue
			}

			log.Info("Lease renewed, new expiration: ", bn.Lease().Expiration)
			dur = bn.Lease().Expiration.Sub(time.Now()) - renewMargin

		case e := <-evts:
			switch e.Type {
			case subnet.EventAdded:
				bn.Lease().Expiration = e.Lease.Expiration
				dur = bn.Lease().Expiration.Sub(time.Now()) - renewMargin
				log.Infof("Waiting for %s to renew lease", dur)

			case subnet.EventRemoved:
				log.Error("Lease has been revoked. Shutting down daemon.")
				return errInterrupted
			}

		case <-ctx.Done():
			log.Infof("Stopped monitoring lease")
			return errCanceled
		}
	}
}


func mustRunHealthz() {
	address := net.JoinHostPort(opts.healthzIP, strconv.Itoa(opts.healthzPort))
	log.Infof("Start healthz server on %s", address)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("flanneld is running"))
	})

	if err := http.ListenAndServe(address, nil); err != nil {
		log.Errorf("Start healthz server error. %v", err)
		panic(err)
	}
}

func ReadCIDRFromSubnetFile(path string, CIDRKey string) ip.IP4Net {
	var prevCIDR ip.IP4Net
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		prevSubnetVals, err := godotenv.Read(path)
		if err != nil {
			log.Errorf("Couldn't fetch previous %s from subnet file at %s: %s", CIDRKey, path, err)
		} else if prevCIDRString, ok := prevSubnetVals[CIDRKey]; ok {
			err = prevCIDR.UnmarshalJSON([]byte(prevCIDRString))
			if err != nil {
				log.Errorf("Couldn't parse previous %s from subnet file at %s: %s", CIDRKey, path, err)
			}
		}
	}
	return prevCIDR
}



func handleSubnetEvents(batch []subnet.Event) {
	for _, event := range batch {
		leaseSubnet := event.Lease.Subnet
		leaseAttrs := event.Lease.Attrs
		if !strings.EqualFold(leaseAttrs.BackendType, "vxlan") {
			log.Warningf("ignoring non-vxlan subnet(%v): type=%v", leaseSubnet, leaseAttrs.BackendType)
			continue
		}

		var vxlanAttrs vxlanLeaseAttrs
		if err := json.Unmarshal(leaseAttrs.BackendData, &vxlanAttrs); err != nil {
			log.Error("error decoding subnet lease JSON: ", err)
			continue
		}

		hnsnetwork, err := hcn.GetNetworkByName(nw.dev.link.Name)
		if err != nil {
			log.Errorf("Unable to find network %v, error: %v", nw.dev.link.Name, err)
			continue
		}
		managementIp := event.Lease.Attrs.PublicIP.String()

		networkPolicySettings := hcn.RemoteSubnetRoutePolicySetting{
			IsolationId:                 4096,
			DistributedRouterMacAddress: net.HardwareAddr(vxlanAttrs.VtepMAC).String(),
			ProviderAddress:             managementIp,
			DestinationPrefix:           event.Lease.Subnet.String(),
		}
		rawJSON, err := json.Marshal(networkPolicySettings)
		networkPolicy := hcn.NetworkPolicy{
			Type:     hcn.RemoteSubnetRoute,
			Settings: rawJSON,
		}

		policyNetworkRequest := hcn.PolicyNetworkRequest{
			Policies: []hcn.NetworkPolicy{networkPolicy},
		}

		switch event.Type {
		case subnet.EventAdded:
			for _, policy := range hnsnetwork.Policies {
				if policy.Type == hcn.RemoteSubnetRoute {
					existingPolicySettings := hcn.RemoteSubnetRoutePolicySetting{}
					err = json.Unmarshal(policy.Settings, &existingPolicySettings)
					if err != nil {
						log.Error("Failed to unmarshal settings")
					}
					if existingPolicySettings.DestinationPrefix == networkPolicySettings.DestinationPrefix {
						existingJson, err := json.Marshal(existingPolicySettings)
						if err != nil {
							log.Error("Failed to marshal settings")
						}
						existingPolicy := hcn.NetworkPolicy{
							Type:     hcn.RemoteSubnetRoute,
							Settings: existingJson,
						}
						existingPolicyNetworkRequest := hcn.PolicyNetworkRequest{
							Policies: []hcn.NetworkPolicy{existingPolicy},
						}
						hnsnetwork.RemovePolicy(existingPolicyNetworkRequest)
					}
				}
			}
			if networkPolicySettings.DistributedRouterMacAddress != "" {
				hnsnetwork.AddPolicy(policyNetworkRequest)
			}
		case subnet.EventRemoved:
			if networkPolicySettings.DistributedRouterMacAddress != "" {
				hnsnetwork.RemovePolicy(policyNetworkRequest)
			}
		default:
			log.Error("internal error: unknown event type: ", int(event.Type))
		}
	}
}