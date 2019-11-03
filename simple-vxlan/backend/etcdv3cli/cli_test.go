package etcdv3cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cli", func() {
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
	var (
		testVxlan1 = "test.eth1.100"
		testVxlan2 = "test.eth1.200"
		testIPStr1 = "192.168.56.100"
		testIPStr2 = "192.168.56.200"
		testFile1  = filepath.Join(cacheDir, testVxlan1)
		testFile2  = filepath.Join(cacheDir, testVxlan2)
	)

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
		em, _ := etcdv3.New()
		defer em.Close()
		em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
		os.Remove(testFile1)
		os.Remove(testFile2)

	})

	AfterEach(func() {
		em, _ := etcdv3.New()
		defer em.Close()
		em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
		os.Remove(testFile1)
		os.Remove(testFile2)
		os.Setenv("ETCD_CFG_DIR", etcdCfgDir)
		os.Setenv("ETCD_ROOT_DIR", etcdRootDir)
		os.Setenv("HOSTNAME", hostname)
	})

	It("parse key to network ad src", func() {
		testkey := []byte(fmt.Sprintf("test/%s/%s/%s", vxlanKeyDir, testVxlan1, testIPStr1))
		testvalue := []byte("hostname")
		vxlan, src := ParseVxlan(testkey, testvalue)
		Expect(vxlan).To(Equal(testVxlan1))
		Expect(src).To(Equal(testIPStr1))
	})
	It("cache rec", func() {
		err := cacheRec(testVxlan1, testIPStr1)
		Expect(err).To(BeNil())
		err = cacheRec(testVxlan2, testIPStr2)
		Expect(err).To(BeNil())
		value, err := ioutil.ReadFile(testFile1)
		Expect(err).To(BeNil())
		Expect(string(value)).To(Equal(testIPStr1))
		value, err = ioutil.ReadFile(testFile2)
		Expect(err).To(BeNil())
		Expect(string(value)).To(Equal(testIPStr2))
	})
	It("send cache to etcd ", func() {
		cacheRec(testVxlan1, testIPStr1)
		cacheRec(testVxlan2, testIPStr2)
		_, err := ioutil.ReadFile(testFile1)
		Expect(err).To(BeNil())
		_, err = ioutil.ReadFile(testFile2)
		Expect(err).To(BeNil())
		testMap := map[string]string{testVxlan1: testIPStr1, testVxlan2: testIPStr2}
		em, _ := etcdv3.New()
		defer em.Close()
		err = CacheToEtcd()
		Expect(err).To(BeNil())
		keyDir := filepath.Join(em.RootKeyDir, vxlanKeyDir)
		ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
		resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
		cancel()
		Expect(len(resp.Kvs)).To(Equal(2))
		for _, ev := range resp.Kvs {
			vxlan, src := ParseVxlan(ev.Key, ev.Value)
			v, ok := testMap[vxlan]
			Expect(ok).To(BeTrue())
			Expect(v).To(Equal(src))
		}
		_, err = ioutil.ReadFile(testFile1)
		Expect(err).To(HaveOccurred())
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = ioutil.ReadFile(testFile2)
		Expect(err).To(HaveOccurred())
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

})
