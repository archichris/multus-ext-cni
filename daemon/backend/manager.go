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
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/subnet"
)

var constructors = make(map[string]BackendCtor)

type Manager interface {
	GetBackend(backendType string) (Backend, error)
}

type Network struct{
    config   *subnet.Config
	extIface *ExternalInterface
	bn       subnet.Network
}


// type manager struct {
// 	ctx      context.Context
// 	sm       subnet.Manager
// 	extIface *ExternalInterface
// 	mux      sync.Mutex
// 	active   map[string]Backend
// 	wg       sync.WaitGroup
// }

type manager struct {
	ctx      context.Context
	sm       subnet.Manager
	// extIface *ExternalInterface
	mux      sync.Mutex
	active   map[string]BackendInstance
	wg       sync.WaitGroup
	dur      time.Minute
	keyDir   string
}



func NewManager(ctx context.Context, sm subnet.Manager, wg sync.WaitGroup) Manager {
	return &manager{
		ctx:      ctx,
		sm:       sm,
		active:   make(map[string]BackendInstance),
		wg:  wg
		dur:  60 * time.Minute
		keyPrefix: "multus"
	}
}

func (m *manager) Run(){
	m.wg.Add(2)
	go func() {
		m.MonitorCfg(ctx)
		wg.Done()
	}()

	go func() {
		m.MonitorLease(ctx)
		wg.Done()
	}()
	for {
		select {
		case evtBatchs := <-events:
			for _, batch := range evtBatchs{}
				switch batch.MsgType {
				case SubnetMsg:
					nm.handleSubnetEvents(batch)
				case NetMsg:
					nm.handleNetEvents(batch)
				default:
					log.error("invalid msgtype %s")
				}
		case <-ctx.Done():
			return
		}
	}
}



func Register(name string, ctor BackendCtor) {
	constructors[name] = ctor
}


func (bm *manager)AddNet(config *subnet.Config) error{
	bm.mux.Lock()
	defer bm.mux.Unlock()

	betype := strings.ToLower(config.backendType)
	// activeName = betype + "_" + netName
	// see if one is already running
	if ins, ok := bm.active[config.Name]; ok {
		// todo check if the config is the same with the exist network
		return nil
	}

    extIface, err := LookupExtIface(config.Iface, "")
	if err != nil {
		log.Infof("Could not find valid interface matching %s: %s", iface, err)
	}


	// first request, need to create and run it
	befunc, ok := constructors[betype]
	if !ok {
		return nil, fmt.Errorf("unknown backend type: %v", betype)
	}

	be, err := befunc(bm.sm, extIface)
	if err != nil {
		return nil, err
	}

	bn, err := be.RegisterNetwork(bm.ctx, bm.wg, config)
	if err != nil {
		log.Errorf("Error registering network: %s", err)
		return fmt.Errorf("Error fetching backend: %s", err)
	}

	bm.active[netName] = BackendInstance{config: config, extIface: extIface, bn: bn}

	if err := WriteSubnetFile(config.Name); err != nil {
		// Continue, even though it failed.
		log.Warningf("Failed to write subnet file: %s", err)
	} else {
		log.Infof("Wrote subnet file to %s", opts.subnetFile)
	}
}

func (bm *manager)DelNet(netName string) error{
	// Work out which interface to use
	var err error

	bm.mux.Lock()
	defer bm.mux.Unlock()

	ins, ok := bm.active[netName]
	if !ok {
		return nil
	}

	//todo do graceful deletion
	delete(bm.active, netName)
	return os.Remove(filepath.Join(bm.dir, netName))
}


func (bm *manager)WriteNetFile(netName string) error {
	tempFile := filepath.Join(bm.dir, "." + netName)
	f, err := os.Create(tempFile)
	if err != nil {
		return err
	}

	ins, ok := bm.active[netName]
	if !ok {
		log.Errorf("network %s not exsit", netName)
		return fmt.Errorf("network %s not exsit", netName)
	}


	// Write out the first usable IP by incrementing
	// sn.IP by one
	sn := ins.bn.Lease().Subnet
	sn.IP += 1

	fmt.Fprintf(f, "Name=%s\n", ins.config.Network)
	fmt.Fprintf(f, "TYPE=%\n", ins.beType)
	fmt.Fprintf(f, "SUBNET=%s\n", sn)
	fmt.Fprintf(f, "MTU=%d\n", bn.MTU())
	_, err = fmt.Fprintf(f, "IPMASQ=%v\n", ipMasq)
	f.Close()
	if err != nil {
		return err
	}

	// rename(2) the temporary file to the desired location so that it becomes
	// atomically visible with the contents
	return os.Rename(tempFile, filepath.Join(bm.dir, netName))
	//TODO - is this safe? What if it's not on the same FS?
}


func (bm *manager)MonitorLease() error {
	for {
		select {
		case <-time.After(bm.dur):
			for netName, ins := range bm.active{
				if ins.bn.Lease().Expiration.Sub(time.Now()) < bm.dur * 2 {
					bm.sm.RenewLease(ctc, ins)
				}
			}
		}
		case <-ctx.Done():
			log.Infof("Stopped monitoring lease")
			return errCanceled
		}
	}
}

func (bm *manager)MonitorCfg() error {
	cli,err := bm.sm.NewClient()
    if err != nil {
		log.Errorf("client create failed, %v", err)
		panic(err)
	}
	rch := cli.Watch(bm.ctx, bm.keyPrefix, clientv3.WithPrefix())
	for wresp := range rch {
		for _, ev := range wresp.Events {
			fmt.Printf("Watch: %s %q: %q \n", ev.Type, ev.Kv.Key, ev.Kv.Value)
		}
	}
}