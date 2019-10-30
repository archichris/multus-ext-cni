package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	// "github.com/containernetworking/cni/pkg/types"
	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/intel/multus-cni/multus-ipam/backend/etcdv3cli"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apiv1 "k8s.io/api/core/v1"
)

var _ = Describe("Controller", func() {
	var etcdCfgDir, etcdRootDir, hostname, kubeConf string
	// idCfg := []byte("node201")
	var etcdCfg = []byte(`
{
    "name": "multus-etcdcni",
    "endpoints": ["192.168.56.201:12379"],
    "auth": {
        "client": {
		"secureTransport": false,
		"enableAuthentication": false,
		"secretDirectory": "/etc/cni/net.d/multus.d/etcd/pki"
		},
		"peer": {
		"secureTransport": false,
		"enableAuthentication": false,
		"useAutoTLS": false
		}
	}
}
`)

	var cniCfg = []byte(`
{
	"Name": "testnet",
	"cniVersion": "0.3.0",
	"type": "multus-vxlan",
	"master": "eth1",
	"ipMasq": true,
	"IsGw": false,
	"IsDefaultGw": false,
	"hairpinMode": true,
	"vlan": 0,
	"vxlan": {
		"vxlanId": 201,
		"port": 8472,
		"learning": false,
		"gbp": false
	},
	"ipam": {
		"type": "multus-ipam",
		"ranges": [
			[
				{
					"subnet": "192.168.56.0/24",
					"rangeStart": "192.168.56.32",
					"rangeEnd": "192.168.56.159",
					"gateway": "192.168.56.1",
					"reserves": [
						"192.168.56.0",
						"192.168.56.255"
					]
				}
			]
		],
		"applyUnit": 4,
		"routes": [
			{
				"dst": "0.0.0.0/0"
			}
		]
	}
}
`)

	var cniFixCfg = []byte(`
{
	"Name": "testfixnet",
	"cniVersion": "0.3.0",
	"type": "multus-vxlan",
	"master": "eth1",
	"ipMasq": true,
	"IsGw": false,
	"IsDefaultGw": false,
	"hairpinMode": true,
	"vlan": 0,
	"vxlan": {
		"vxlanId": 201,
		"port": 8472,
		"learning": false,
		"gbp": false
	},
	"ipam": {
		"type": "multus-ipam",
		"ranges": [
			[
				{
					"subnet": "192.168.56.0/24",
					"rangeStart": "192.168.56.128",
					"rangeEnd": "192.168.56.255",
					"gateway": "192.168.56.1",
					"reserves": [
						"192.168.56.0",
						"192.168.56.255"
					]
				}
			]
		],
		"fix": true,
		"routes": [
			{
				"dst": "0.0.0.0/0"
			}
		]
	}
}
`)
	// var subnet, _ = types.ParseCIDR("192.168.56.0/24")
	// var rangeTest = allocator.Range{Subnet: *(*types.IPNet)(subnet)}

	BeforeEach(func() {
		etcdCfgDir = os.Getenv("ETCD_CFG_DIR")
		etcdRootDir = os.Getenv("ETCD_ROOT_DIR")
		hostname = os.Getenv("HOSTNAME")
		kubeConf = os.Getenv("KUBE_CONFIG")
		ioutil.WriteFile("/tmp/etcd.conf", etcdCfg, 0666)
		os.Setenv("ETCD_CFG_DIR", "/tmp")
		os.Setenv("ETCD_ROOT_DIR", "test")
		os.Setenv("HOSTNAME", "hostname")
		os.Setenv("KUBE_CONFIG", "/etc/cni/net.d/multus.d/multus.kubeconfig")
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
		em, _ := etcdv3.New()
		defer em.Close()
		em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
	})

	AfterEach(func() {
		em, _ := etcdv3.New()
		defer em.Close()
		em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
		os.Setenv("ETCD_CFG_DIR", etcdCfgDir)
		os.Setenv("ETCD_ROOT_DIR", etcdRootDir)
		os.Setenv("HOSTNAME", hostname)
		os.Setenv("KUBE_CONFIG", kubeConf)

	})
	It("handle node delete", func() {
		em, _ := etcdv3.New()
		defer em.Close()
		netConf := allocator.Net{}
		json.Unmarshal(cniCfg, &netConf)
		n := 3
		for i := 0; i < n; i++ {
			etcdv3cli.IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
		}
		keyDir := filepath.Join(em.RootKeyDir, "lease", netConf.Name)

		ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		cancel()
		Expect(len(resp.Kvs)).To(Equal(n))

		ctx, _ = context.WithCancel(context.Background())
		wg := sync.WaitGroup{}
		km, err := NewKubeManager(ctx, wg)
		Expect(err).To(BeNil())

		node := apiv1.Node{}
		node.Name = em.Id

		err = km.handleNodeDelEvent(&node)
		Expect(err).To(BeNil())
		ctx, cancel = context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		resp, _ = em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		cancel()
		Expect(len(resp.Kvs)).To(Equal(0))
	})
	It("handle fix ip check", func() {
		netConf := allocator.Net{}
		json.Unmarshal(cniFixCfg, &netConf)
		// ipamConf := netConf.IPAM
		// logging.Debugf("netConf:%v", ipamConf)
		em, _ := etcdv3.New()
		defer em.Close()
		keyDir := filepath.Join(em.RootKeyDir, "fix", netConf.Name)
		// ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		// resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		// cancel()
		// for _, ev := range resp.Kvs {
		// 	etcdv3.TransDelKey(em.Cli, string(ev.Key))
		// }
		n := 3
		for i := 0; i < n; i++ {
			_, err := etcdv3cli.IPAMApplyFixIP(netConf.Name, &netConf.IPAM.Ranges[0][0], fmt.Sprintf("default:wahaha%d", i))
			Expect(err).To(BeNil())
		}
		ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		cancel()
		Expect(len(resp.Kvs)).To(Equal(n))

		ctx, _ = context.WithCancel(context.Background())
		wg := sync.WaitGroup{}
		km, err := NewKubeManager(ctx, wg)
		Expect(err).To(BeNil())

		err = km.CheckFixIP()
		Expect(err).To(BeNil())

		ctx, cancel = context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		resp, _ = em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		cancel()
		Expect(len(resp.Kvs)).To(Equal(0))

	})
})
