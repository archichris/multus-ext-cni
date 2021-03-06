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
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/archichris/netools/dev"
	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	ipamDocker "github.com/intel/multus-cni/multus-ipam/backend/dockercli"
	ipamEtcd "github.com/intel/multus-cni/multus-ipam/backend/etcdv3cli"
	vxEtcd "github.com/intel/multus-cni/multus-vxlan/backend/etcdv3cli"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"
)

var (
	defaultWaitTime   = 5 * time.Second
	defaultTickerTime = time.Duration(5+rand.Intn(2)) * time.Minute
	// ipamEtcdCheckTicker  = 1
	// ipamLocalCheckTicker = 10
	// vxEtcdCheckTicker    = 1
)

// var (
// 	errInterrupted = errors.New("interrupted")
// 	errCanceled    = errors.New("canceled")
// )

func init() {
	//for debug
	logFile := os.Getenv("LOG_FILE")
	logLevel := os.Getenv("LOG_LEVEL")

	if len(logFile) > 0 {
		logging.SetLogFile(logFile)
	}

	if len(logLevel) > 0 {
		logging.SetLogLevel(logLevel)
	}

	//debug
	// logging.SetLogFile("/host/var/log/multus-daemon.log")
	// logging.SetLogLevel("debug")
}

type multusd struct {
	ctx    context.Context
	wg     *sync.WaitGroup
	mux    sync.Mutex
	buf    map[string]string
	keyDir string
}

func newMultusd(ctx context.Context, wg *sync.WaitGroup, keyDir string) *multusd {
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

	//todo prevent out of ord between history record and watching
	ipamEtcd.IPAMCheckEtcd()
	tickerTime := defaultTickerTime
	tmp := os.Getenv("TICKER_TIME")
	if tmp != "" {
		t, err := strconv.Atoi(tmp)
		if err == nil {
			tickerTime = time.Duration(t+rand.Intn(int(t/2))) * time.Second
		}
	}
	logging.Verbosef("using ticker time %v", tickerTime)
	ticker := time.NewTicker(tickerTime)
	for {
		select {
		case <-d.ctx.Done():
			logging.Verbosef("ctx stop multusd")
			return
		case <-ticker.C:
			// logging.Debugf("ticker run")
			ipamEtcd.IPAMCheckEtcd()
			ipamDocker.IPAMCheckLocalIPs("")
			vxEtcd.CacheToEtcd()
		}
	}
}

func (d *multusd) Watching(ctx context.Context, keyPrefix string) {
	logging.Verbosef("Watching %v", keyPrefix)
	var cli *clientv3.Client = nil
	for {
		etcdMultus, err := etcdv3.New()
		if cli != nil {
			cli.Close()
		}
		cli = etcdMultus.Cli
		if err != nil {
			logging.Errorf("Create etcd client failed, %v", err)
			time.Sleep(defaultWaitTime)
			continue
		}
		defer cli.Close()
		d.procHistoryRecord("")
		rch := cli.Watch(ctx, keyPrefix, clientv3.WithPrefix())
		for wresp := range rch {
			for _, ev := range wresp.Events {
				logging.Verbosef("Watch: %s %q: %q \n", ev.Type, ev.Kv.Key, ev.Kv.Value)
				name, src := vxEtcd.ParseVxlan(ev.Kv.Key, ev.Kv.Value)
				switch ev.Type.String() {
				case "DELETE":
					d.watchedDelSubnet(name, src)
				case "PUT":
					d.watchedAddSubnet(name, src)
				default:
					logging.Errorf("unexpected operate %s", ev.Type)
				}
			}
		}
	}
}

func (d *multusd) procHistoryRecord(vx string) error {
	logging.Verbosef("procHistoryRecord %v, %d", vx, len(vx))
	etcdMultus, err := etcdv3.New()
	cli := etcdMultus.Cli
	if err != nil {
		return logging.Errorf("Create etcd client failed, %v", err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	getResp, err := cli.Get(ctx, d.keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return logging.Errorf("Get %v failed, %v", d.keyDir, err)
	}
	for _, ev := range getResp.Kvs {
		logging.Verbosef("process: PUT %q: %q \n", string(ev.Key), string(ev.Value))
		name, src := vxEtcd.ParseVxlan(ev.Key, ev.Value)
		if (len(vx) == 0) || (vx == name) {
			d.watchedAddSubnet(name, src)
		}
	}
	return nil
}

func (d *multusd) watchedAddSubnet(name, src string) error {
	l, err := netlink.LinkByName(name)
	if err != nil {
		//It may be nomral when no container run in this node
		logging.Verbosef("get interface %v failed, %v", name, err)
		d.buf[name] = name
		return nil
	}

	if _, ok := d.buf[name]; ok {
		delete(d.buf, name)
		return d.procHistoryRecord(name)
	}

	vx, ok := l.(*netlink.Vxlan)
	if !ok {
		return logging.Errorf("%s already exists but is not a vxlan", name)
	}

	if vx.SrcAddr.String() == src {
		logging.Verbosef("get record of self %s, nothing need to do", src)
		return nil
	}

	defaultMac := net.HardwareAddr{0, 0, 0, 0, 0, 0}

	err = dev.AddFDB(vx.Index, defaultMac, net.ParseIP(src))
	if err != nil {
		return logging.Errorf("Add fdb %v, %v, %v failed, %v", vx.Index, defaultMac, src, err)
	}
	return nil
}

func (d *multusd) watchedDelSubnet(name, src string) error {
	l, err := netlink.LinkByName(name)
	if err != nil {
		//It may be nomral when no container run in this node
		logging.Verbosef("get interface %v failed, %v", name, err)
		return nil
	}

	vx, ok := l.(*netlink.Vxlan)
	if !ok {
		return logging.Errorf("%s already exists but is not a vxlan", name)
	}

	if vx.SrcAddr.String() == src {
		logging.Verbosef("get record of self:%s, nothing need to do", src)
		return nil
	}

	defaultMac := net.HardwareAddr{0, 0, 0, 0, 0, 0}
	err = dev.DelFDB(vx.Index, defaultMac, net.ParseIP(src))
	if err != nil {
		return logging.Errorf("Add fdb %v, %v, %v failed, %v", vx.Index, defaultMac, src, err)
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
		newMultusd(ctx, &wg, "multus/vxlan").Run()
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
