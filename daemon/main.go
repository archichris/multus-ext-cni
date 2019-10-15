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
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/dev"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/etcdv3cli"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"
)

const requestTimeout = 5 * time.Second

// var (
// 	errInterrupted = errors.New("interrupted")
// 	errCanceled    = errors.New("canceled")
// )

func init() {
	//for debug
	logging.SetLogFile("/tmp/multus-daemon.log")
	logging.SetLogLevel("debug")
}

type vxlan struct {
	index int
	name  string
}

type multusd struct {
	ctx    context.Context
	wg     sync.WaitGroup
	mux    sync.Mutex
	buf    map[string]string
	keyDir string
}

func newMultusd(ctx context.Context, wg sync.WaitGroup, keyDir string) *multusd {
	return &multusd{
		ctx:    ctx,
		wg:     wg,
		keyDir: keyDir,
		buf:    make(map[string]string),
	}
}

func (d *multusd) Run() {
	//TODO define even type
	// events := make(chan []string)
	logging.Verbosef("multusd is running...")
	d.wg.Add(1)
	go func() {
		d.Watching(d.ctx, d.keyDir)
		logging.Verbosef("Watching exited")
		d.wg.Done()
	}()
	d.procHistoryRecord(d.keyDir)
	//todo prevent out of ord between history record and watching
	for {
		select {
		case <-d.ctx.Done():
			logging.Verbosef("ctx stop multusd")
			return
		}
	}
}

func (d *multusd) Watching(ctx context.Context, keyPrefix string) {
	logging.Verbosef("Watching %v", keyPrefix)
	cli, _, err := etcdv3.NewClient()
	if err != nil {
		logging.Panicf("Create etcd client failed, %v", err)
	}
	defer cli.Close()
	rch := cli.Watch(ctx, keyPrefix, clientv3.WithPrefix())
	for wresp := range rch {
		for _, ev := range wresp.Events {
			logging.Verbosef("Watch: %s %q: %q \n", ev.Type, ev.Kv.Key, ev.Kv.Value)
			s, err := etcdv3cli.IpamGetLeaseInfo(ev.Kv.Key, ev.Kv.Value)
			if err != nil {
				continue
			}
			switch ev.Type.String() {
			case "DELETE":
				d.watchedDelSubnet(s)
			case "PUT":
				d.watchedAddSubnet(s)
			default:
				logging.Errorf("unexpected operate %s", ev.Type)
			}
		}
	}
}

func (d *multusd) procHistoryRecord(vx string) error {
	logging.Verbosef("Watching %v", d.keyDir)
	cli, _, err := etcdv3.NewClient()
	if err != nil {
		logging.Panicf("Create etcd client failed, %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	getResp, err := cli.Get(ctx, d.keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return logging.Errorf("Get %v failed, %v", d.keyDir, err)
	}
	for _, ev := range getResp.Kvs {
		logging.Verbosef("process: PUT %q: %q \n", ev.Key, ev.Value)
		s, err := etcdv3cli.IpamGetLeaseInfo(ev.Key, ev.Value)
		if err != nil {
			continue
		}
		if (len(vx) == 0) || (vx == s.V.Vxlan.Name) {
			d.watchedAddSubnet(s)
		}
	}
	return nil
}

func (d *multusd) watchedDelSubnet(s *etcdv3cli.Lease) error {
	if s.V.Vxlan == nil {
		return nil
	}
	l, err := netlink.LinkByName(s.V.Vxlan.Name)
	if err != nil {
		//It may be nomral when no container run in this node
		logging.Verbosef("get interface %v failed, %v", s.V.Vxlan.Name, err)
		return nil
	}

	vx, ok := l.(*netlink.Vxlan)
	if !ok {
		return logging.Errorf("%s already exists but is not a vxlan", s.V.Vxlan.Name)
	}

	if vx.SrcAddr.String() == s.V.M.IP {
		logging.Verbosef("get record of self, nothing need to do")
		return nil
	}

	defaultMac := net.HardwareAddr{0, 0, 0, 0, 0, 0}
	err = dev.DelFDB(vx.Index, defaultMac, net.ParseIP(s.V.M.IP))
	if err != nil {
		return logging.Errorf("Add fdb %v, %v, %v failed, %v", vx.Index, defaultMac, s.V.M.IP, err)
	}
	return nil
}

func (d *multusd) watchedAddSubnet(s *etcdv3cli.Lease) error {
	// err := netlink.RouteAdd(&netlink.Route{
	// 	LinkIndex: iface.Index,
	// 	Scope:     netlink.SCOPE_UNIVERSE,
	// 	Dst:       &l.K.N,
	// 	Gw:        net.ParseIP(l.V.M.IP),
	// })
	// if err != nil {
	// 	return logging.Errorf("RouteAdd link %v to %v via %v failed, %v", iface.Index, l.K.N, l.V.M.IP, err)
	// }
	if s.V.Vxlan == nil {
		return nil
	}

	l, err := netlink.LinkByName(s.V.Vxlan.Name)
	if err != nil {
		//It may be nomral when no container run in this node
		logging.Verbosef("get interface %v failed, %v", s.V.Vxlan.Name, err)
		d.buf[s.V.Vxlan.Name] = s.V.Vxlan.Name
		return nil
	}

	if _, ok := d.buf[s.V.Vxlan.Name]; ok {
		delete(d.buf, s.V.Vxlan.Name)
		return d.procHistoryRecord(s.V.Vxlan.Name)
	}

	vx, ok := l.(*netlink.Vxlan)
	if !ok {
		return logging.Errorf("%s already exists but is not a vxlan", s.V.Vxlan.Name)
	}

	if vx.SrcAddr.String() == s.V.M.IP {
		logging.Verbosef("get record of self, nothing need to do")
		return nil
	}

	defaultMac := net.HardwareAddr{0, 0, 0, 0, 0, 0}

	err = dev.AddFDB(vx.Index, defaultMac, net.ParseIP(s.V.M.IP))
	if err != nil {
		return logging.Errorf("Add fdb %v, %v, %v failed, %v", vx.Index, defaultMac, s.V.M.IP, err)
	}
	return nil
}

func main() {
	// install signal process
	logging.Verbosef("install signals")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		shutdownHandler(ctx, sigs, cancel)
		logging.Verbosef("shutdownHandler terminated")
		wg.Done()
	}()

	wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		newMultusd(ctx, wg, "multus/multus-vxlan").Run()
		wg.Done()
	}()

	logging.Verbosef("Waiting for all goroutines to exit")
	// Block waiting for all the goroutines to finish.
	wg.Wait()
	logging.Verbosef("Exiting cleanly...")
	os.Exit(0)
}

func shutdownHandler(ctx context.Context, sigs chan os.Signal, cancel context.CancelFunc) {
	// Wait for the context do be Done or for the signal to come in to shutdown.
	select {
	case <-ctx.Done():
		logging.Verbosef("Stopping shutdownHandler...")
	case <-sigs:
		// Call cancel on the context to close everything down.
		cancel()
		logging.Verbosef("shutdownHandler sent cancel signal...")
	}

	// Unregister to get default OS nuke behaviour in case we don't exit cleanly
	signal.Stop(sigs)
}
