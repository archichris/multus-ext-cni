package etcdv3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"io/ioutil"
	"strings"
	"context"
	"path/filepath"
	"github.com/intel/multus-cni/logging"
)

var _ = Describe("Etcdv3", func() {
	var etcdCfgDir, etcdRootDir, hostname string
	idCfg :=[]byte("node201")
	etcdCfg :=[]byte(`
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

	BeforeEach(func() {
		etcdCfgDir = os.Getenv("ETCD_CFG_DIR")
		etcdRootDir = os.Getenv("ETCD_ROOT_DIR")
		hostname = os.Getenv("HOSTNAME")
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
	})

	AfterEach(func() {
		os.Setenv("ETCD_CFG_DIR", etcdCfgDir)
		os.Setenv("ETCD_ROOT_DIR", etcdRootDir)
		os.Setenv("HOSTNAME", hostname)
	})

	Describe("Parameters initilization", func() {
		Context("using parameters from environment", func() {
			It("should read parameters from env sucessfully", func() {
				os.Setenv("ETCD_CFG_DIR", "etcd_cfg_dir")
				os.Setenv("ETCD_ROOT_DIR", "etcd_root_dir")
				os.Setenv("HOSTNAME", "hostname")
				etcdCfgDir, rootKeyDir, id := getInitParams()
				Expect(etcdCfgDir).To(Equal("etcd_cfg_dir"))
				Expect(rootKeyDir).To(Equal("etcd_root_dir"))
				Expect(id).To(Equal("hostname"))
			})
		})
		Context("using default value", func() {
			It("cfg dir and root dir should use default parameters", func() {
				os.Setenv("ETCD_CFG_DIR", "")
				os.Setenv("ETCD_ROOT_DIR", "")
				os.Setenv("HOSTNAME", "")
				etcdCfgDir, rootKeyDir, _ := getInitParams()
				Expect(etcdCfgDir).To(Equal(defaultEtcdCfgDir))
				Expect(rootKeyDir).To(Equal(defaultEtcdRootDir))
			})
			It("id should use default parameters", func() {
				os.Setenv("ETCD_ROOT_DIR", "/tmp")
				idFile := filepath.Join("/tmp","id")
				ioutil.WriteFile(idFile,idCfg,0666)
				_, _, id := getInitParams()
				Expect(id).To(Equal(strings.Trim(string(idCfg)," \r\n\t")))
				os.Remove(idFile)
			})
		})
	})

	Describe("Get etcd configuration", func() {
		Context("read and parse correct cfg", func() {
			It("should read and parse cfg correctly", func() {
				ioutil.WriteFile("/tmp/etcd.conf", etcdCfg, 0666)
				cfg, err := getEtcdCfg("/tmp/etcd.conf")
				Expect(err==nil).To(Equal(true))
				Expect(cfg!=nil).To(Equal(true))
				os.Remove("/tmp/etcd.conf")
			})
		})
		Context("read and parse error cfg", func() {
			It("should return error when cfg does not exsit", func() {
				os.Remove("/tmp/ghost.conf")
				cfg, err := getEtcdCfg("/tmp/tmp.conf")
				Expect(err!=nil).To(Equal(true))
				Expect(cfg==nil).To(Equal(true))
			})
			It("should return error when cfg is not correct", func() {
				ioutil.WriteFile("/tmp/error.conf", []byte("errdfjdgjfam03giov0iND;DSKF21XDSFH"), 0666)
				cfg, err := getEtcdCfg("/tmp/error.conf")
				Expect(err!=nil).To(Equal(true))
				Expect(cfg==nil).To(Equal(true))
			})
		})
	})

	Describe("New etcd client without ca", func() {
		Context("create etcd client with correct cfg", func() {
			It("should create etcd client successfully ", func() {
				// the etcd config should be correct in cfg file
				ioutil.WriteFile("/tmp/etcd.conf", etcdCfg, 0666)
				os.Setenv("ETCD_CFG_DIR", "/tmp")
				os.Setenv("ETCD_ROOT_DIR", "etcd_root_dir")
				os.Setenv("HOSTNAME", "hostname")	
				etcdMultus, err := New()
				cli, rKeyDir, id := etcdMultus.Cli, etcdMultus.RootKeyDir, etcdMultus.Id
				Expect(err==nil).To(Equal(true))
				Expect(cli!=nil).To(Equal(true))
				Expect(rKeyDir).To(Equal("etcd_root_dir"))
				Expect(id).To(Equal("hostname"))
				cli.Close()
				os.Remove("/tmp/etcd.conf")
			})
		})
		Context("create etcd client with error cfg", func() {
			It("should create etcd client failed ", func() {
				// the etcd config should be correct in cfg file
				os.Remove("/tmp/etcd.conf")
				os.Setenv("ETCD_CFG_DIR", "/tmp/e")
				os.Setenv("ETCD_ROOT_DIR", "etcd_root_dir")
				os.Setenv("HOSTNAME", "hostname")	
				etcdMultus, err := New()
				Expect(err!=nil).To(Equal(true))
				Expect(etcdMultus==nil).To(Equal(true))
			})
		})
	})
	Describe("New etcd client with ca", func() {
		Context("create etcd client with correct ca", func() {
			It("should create etcd client successfully ", func() {
				//todo test when the ca function finished
				Expect("a"=="A").To(Equal(false))
			})
		})
	})
	Describe("Get mutex key dir from key", func() {
		Context("input a key and get the mutex", func() {
			It("should get mutex correctly ", func() {
			    mutex := KeyToMutex("multus/type/network/key")
				Expect(mutex).To(Equal("multus/mutex/type/network"))
			})
		})
	})

	Describe("Transaction put and delete in etcd", func() {
		Context("transaction put and delete a key ", func() {
			BeforeEach(func() {
				ioutil.WriteFile("/tmp/etcd.conf", etcdCfg, 0666)
				os.Setenv("ETCD_CFG_DIR", "/tmp")	
				etcdMultus, err := New()
				if err != nil{
					logging.Panicf("new failed, %v", err)
				}
				cli, rKeyDir := etcdMultus.Cli, etcdMultus.RootKeyDir
				defer cli.Close()
				keyDir := filepath.Join(rKeyDir, "testtype","testnet")
				testKey := filepath.Join(keyDir, "transtest")
				TransDelKey(cli, testKey)	
			})

			AfterEach(func(){
				os.Remove("/tmp/etcd.conf")
			})

			It("add and del a key with empty input cli", func() {	
				etcdMultus, err := New()
				cli, rKeyDir := etcdMultus.Cli, etcdMultus.RootKeyDir
				defer cli.Close()
				keyDir := filepath.Join(rKeyDir, "testtype","testnet")
				testKey := filepath.Join(keyDir, "transtest")
				err = TransPutKey(nil, testKey, testKey, false)
				Expect(err==nil).To(Equal(true))
				ctx, cancel := context.WithTimeout(context.Background(), RequestTimeout)
				resp, err := cli.Get(ctx, testKey)	
				if err != nil{
					logging.Errorf("get %v failed, %v", testKey, err)
					return
				}				
				cancel()
				Expect(len(resp.Kvs)).To(Equal(1))		
				Expect(string(resp.Kvs[0].Key)).To(Equal(testKey))	
				Expect(string(resp.Kvs[0].Value)).To(Equal(testKey))
				err = TransPutKey(nil, testKey, testKey, true)
				Expect(err!=nil).To(Equal(true))
				Expect(strings.Contains(err.Error(),"exist")).To(Equal(true))
			})
			It("add and del a key with an valid input cli", func() {	
				etcdMultus, err := New()
				cli, rKeyDir := etcdMultus.Cli, etcdMultus.RootKeyDir
				defer cli.Close()
				keyDir := filepath.Join(rKeyDir, "testtype","testnet")
				testKey := filepath.Join(keyDir, "transtest")
				err = TransPutKey(cli, testKey, testKey, false)
				Expect(err==nil).To(Equal(true))
				Expect(cli!=nil).To(Equal(true))
				ctx, cancel := context.WithTimeout(context.Background(), RequestTimeout)
				resp, err := cli.Get(ctx, testKey)	
				if err != nil{
					logging.Errorf("get %v failed, %v", testKey, err)
					return
				}				
				cancel()
				Expect(len(resp.Kvs)).To(Equal(1))		
				Expect(string(resp.Kvs[0].Key)).To(Equal(testKey))	
				Expect(string(resp.Kvs[0].Value)).To(Equal(testKey))
			})
		})
		Context("batch del keys from etcd batchly", func() {
			It("should del all keys correctly ", func() {
			    
			})
		})
		
	})
})
