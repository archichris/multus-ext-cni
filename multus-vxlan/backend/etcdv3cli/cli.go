package etcdv3cli

import (
	"context"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/vishvananda/netlink"
)

var (
	rootKeyDir     = "multus" //multus/netowrkname/key(ipsegment):value(node)
	requestTimeout = 5 * time.Second
	maxApplyTry    = 3
)

func RecVxlan(network string, vxlan *netlink.Vxlan) error {
	etcdMultus, err := etcdv3.New()
	if err != nil {
		return err
	}
	cli, id:= etcdMultus.Cli, etcdMultus.Id
	defer cli.Close() // make sure to close the client

	s, err := concurrency.NewSession(cli)
	if err != nil {
		return logging.Errorf("create etcd session failed, %v", err)
	}
	defer s.Close()

	key := rootKeyDir + "/vxlan/" + vxlan.Attrs().Name + "/" + vxlan.SrcAddr.String()
	keyMutex := rootKeyDir + "/mutex/vxlan/" + vxlan.Attrs().Name
	value := id

	m := concurrency.NewMutex(s, keyMutex)

	// acquire lock for s
	if err := m.Lock(context.TODO()); err != nil {
		return logging.Errorf("get etcd locd failed, %v", err)
	}

	defer func() {
		if err := m.Unlock(context.TODO()); err != nil {
			logging.Debugf("unlock etcd mutex failed, %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	getResp, err := cli.Get(ctx, key)
	cancel()
	if len(getResp.Kvs) > 0 {
		return nil
	}
	_, err = cli.Put(context.TODO(), key, value)
	if err != nil {
		return logging.Errorf("write key %v to %v failed", key, value)
	}
	return nil
}

func ParseVxlan(key, value []byte) (string, string) {
	k := string(key)
	name := k[:strings.LastIndex(k, "/")]
	name = name[strings.LastIndex(name, "/")+1:]
	src := k[strings.LastIndex(k, "/")+1:]
	return name, src
}
