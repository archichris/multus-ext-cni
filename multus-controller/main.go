// Copyright 2016 flannel authors
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
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	apiv1 "k8s.io/api/core/v1"

	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	resyncPeriod              = 5 * time.Minute
	nodeControllerSyncTimeout = 10 * time.Minute
	tickerTime                = 1 * time.Minute //todo set to a longer time after testing
)

type KubeManager struct {
	client         kubernetes.Interface
	nodeController cache.Controller
	ctx            context.Context
	wg             sync.WaitGroup
	fullCheck      bool
}

func init() {
	//for debug
	logging.SetLogFile("/host/var/log/multus-controller.log")
	logging.SetLogLevel("debug")
}

func NewKubeManager(ctx context.Context, wg sync.WaitGroup) (*KubeManager, error) {
	var km KubeManager
	kubeConfig := os.Getenv("KUBE_CONFIG")

	// uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, logging.Errorf("GetK8sClient: failed to get context for the kubeconfig %v, refer Multus README.md for the usage guide: %v", kubeConfig, err)
	}

	// Specify that we use gRPC
	config.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	config.ContentType = "application/vnd.kubernetes.protobuf"

	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	km.client = client
	_, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return km.client.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return km.client.CoreV1().Nodes().Watch(options)
			},
		},
		&apiv1.Node{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj interface{}) {
				node, isNode := obj.(*apiv1.Node)
				// We can get DeletedFinalStateUnknown instead of *api.Node here and we need to handle that correctly.
				if !isNode {
					deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
					if !ok {
						logging.Verbosef("Error received unexpected object: %v", obj)
						return
					}
					node, ok = deletedState.Obj.(*apiv1.Node)
					if !ok {
						logging.Verbosef("Error deletedFinalStateUnknown contained non-Node object: %v", deletedState.Obj)
						return
					}
				}
				km.handleNodeDelEvent(node)
			},
		},
		// cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	km.nodeController = controller
	// km.nodeStore = listers.NewNodeLister(indexer)
	km.ctx = ctx
	km.wg = wg
	return &km, nil
}

func (km *KubeManager) WatchNode() {
	logging.Verbosef("KubeManager is running...")
	km.nodeController.Run(km.ctx.Done())
	logging.Verbosef("KubeManager is exiting...")
}

func (km *KubeManager) handleNodeDelEvent(n *apiv1.Node) error {
	id := strings.Trim(n.Name, " \n\r\t")
	logging.Verbosef("Node %v is deleted", id)
	em, err := etcdv3.New()
	if err != nil {
		km.fullCheck = true
		return logging.Errorf("Create etcd client failed, %v", err)
	}
	defer em.Close()

	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	getResp, err := em.Cli.Get(ctx, em.RootKeyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return logging.Errorf("Get %v failed, %v", em.RootKeyDir, err)
	}

	delList := []string{}

	for _, ev := range getResp.Kvs {
		v := strings.Trim(string(ev.Value), " \r\n\t")
		logging.Debugf("Key:%v, Value:%v, ID:%v, match:%v ", string(ev.Key), string(ev.Value), id, id == v)
		if v == id {
			delList = append(delList, string(ev.Key))
		}
	}

	if len(delList) > 0 {
		logging.Debugf("Going to del %v", delList)
		etcdv3.TransDelKeys(em.Cli, delList)
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
	km, err := NewKubeManager(ctx, wg)
	if err == nil {
		wg.Add(1)
		go func() {
			km.WatchNode()
			wg.Done()
		}()
		wg.Add(1)
		go func() {
			km.PeriodChkFixIP()
			wg.Done()
		}()
	} else {
		logging.Errorf("create kube manager failed, %v", err)
	}

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

func (km *KubeManager) PeriodChkFixIP() {
	ticker := time.NewTicker(tickerTime)
	for {
		select {
		case <-km.ctx.Done():
			logging.Verbosef("ctx stop multusd")
			return
		case <-ticker.C:
			logging.Debugf("ticker run")
			km.CheckFixIP()
		}
	}
}

func (km *KubeManager) CheckFixIP() error {
	em, err := etcdv3.New()
	if err != nil {
		return logging.Errorf("Create etcd client failed, %v", err)
	}
	defer em.Close()
	fixKeyDir := filepath.Join(em.RootKeyDir, "fix")
	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	getResp, err := em.Cli.Get(ctx, fixKeyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return logging.Errorf("Get %v failed, %v", em.RootKeyDir, err)
	}

	delList := []string{}
	for _, ev := range getResp.Kvs {

		v := strings.Split(strings.Trim(string(ev.Value), " \r\n\t"), ":")
		ns, name := v[0], v[1]
		pod, err := km.client.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})
		if (err != nil) || (pod == nil) {
			delList = append(delList, string(ev.Key))
		}
	}

	if len(delList) > 0 {
		logging.Debugf("Going to del %v", delList)
		etcdv3.TransDelKeys(em.Cli, delList)
	}
	return nil
}
