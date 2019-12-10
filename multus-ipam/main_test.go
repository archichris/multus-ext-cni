package main

import (
	"context"
	"fmt"
	"github.com/containernetworking/cni/pkg/skel"
	// "github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/intel/multus-cni/multus-ipam/backend/disk"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
)

var _ = Describe("Main", func() {
	var etcdCfgDir, etcdRootDir, hostname string
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
	"IsGw": true,
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
					"reserves": [
						"192.168.56.0",
						"192.168.56.255"
					]
				}
			]
		],
		"fixRange": {
			"subnet": "192.168.56.0/24",
			"rangeStart": "192.168.56.128",
			"rangeEnd": "192.168.56.255",
			"reserves": [
				"192.168.56.0",
				"192.168.56.255"
			]
		},
		"fix": true,
		"allocGW": true,
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
	"Name": "testnetfix",
	"cniVersion": "0.3.0",
	"type": "multus-vxlan",
	"master": "eth1",
	"ipMasq": true,
	"IsGw": true,
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
					"reserves": [
						"192.168.56.0",
						"192.168.56.255"
					]
				}
			]
		],
		"fixRange": {
			"subnet": "192.168.56.0/24",
			"rangeStart": "192.168.56.128",
			"rangeEnd": "192.168.56.255",
			"reserves": [
				"192.168.56.0",
				"192.168.56.255"
			]
		},
		"fix": true,
		"allocGW": true,
		"routes": [
			{
				"dst": "0.0.0.0/0"
			}
		]
	}
}
`)
	BeforeEach(func() {
		etcdCfgDir = os.Getenv("ETCD_CFG_DIR")
		etcdRootDir = os.Getenv("ETCD_ROOT_DIR")
		hostname = os.Getenv("HOSTNAME")
		ioutil.WriteFile("/tmp/etcd.conf", etcdCfg, 0666)
		os.Setenv("ETCD_CFG_DIR", "/tmp")
		os.Setenv("ETCD_ROOT_DIR", "test")
		os.Setenv("HOSTNAME", "hostname")
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
		os.Setenv("CNI_PATH", "/tmp")
	})

	AfterEach(func() {
		os.Setenv("ETCD_CFG_DIR", etcdCfgDir)
		os.Setenv("ETCD_ROOT_DIR", etcdRootDir)
		os.Setenv("HOSTNAME", hostname)
		os.Unsetenv("CNI_PATH")
		os.Unsetenv("CNI_ARGS")
	})

	Describe("TODO", func() {
		var netConf *allocator.Net
		BeforeEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
			netConf, _, _ = allocator.LoadIPAMConfig(cniCfg, "")
			s, _ := disk.New(netConf.Name, "")
			caches, _ := s.LoadCache()
			for _, csr := range caches {
				s.DeleteCache(&csr)
			}
		})
		AfterEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
			s, _ := disk.New(netConf.Name, "")
			caches, _ := s.LoadCache()
			for _, csr := range caches {
				s.DeleteCache(&csr)
			}
		})
		It("find first ip range", func() {
			testNS, err := testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())
			args := &skel.CmdArgs{
				ContainerID: "123456789",
				Netns:       testNS.Path(),
				IfName:      "eth0",
				Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s;Fix=testnetfix;Num=testnetfix:4", "testpod", "testnamespace"),
				StdinData:   cniFixCfg,
			}
			err = cmdAdd(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(testNS.Close()).To(Succeed())
		})
	})

})
