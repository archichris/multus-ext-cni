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

	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/etcdv3cli"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"
)

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
	vxlans []vxlan
	keyDir string
}

func newMultusd(ctx context.Context, wg sync.WaitGroup, keyDir string) *multusd {
	return &multusd{
		ctx:    ctx,
		wg:     wg,
		keyDir: keyDir,
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
			l, err := etcdv3cli.IpamGetLeaseInfo(ev.Kv.Key, ev.Kv.Value)
			if err != nil {
				continue
			}
			switch ev.Type.String() {
			case "DELETE":
				d.watchedDelSubnet(l)
			case "PUT":
				d.watchedAddSubnet(l)

			}
		}
	}
}

func (d *multusd) watchedDelSubnet(l *etcdv3cli.Lease) error {
	return nil
}

func (d *multusd) watchedAddSubnet(l *etcdv3cli.Lease) error {
	iface, err := net.InterfaceByName(l.V.M.Name)
	if err != nil {
		return logging.Errorf("get interface %v failed, %v", l.V.M.Name, err)
	}

	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: iface.Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       &l.K.N,
		Gw:        net.ParseIP(l.V.M.IP),
	})
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
