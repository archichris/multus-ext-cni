package etcdv3cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	// "strings"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/coreos/etcd/clientv3"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/ipaddr"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/intel/multus-cni/multus-ipam/backend/disk"
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

	var cniCfg = []byte(`
{
	"Name": "testnet",
	"cniVersion": "0.3.0",
	"type": "multus-vxlan",
	"master": "eth1",
	"ipMasq": true,
	"isGateway": true,
	"isDefaultGateway": false,
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
		"allocGW": true,
		"routes": [
			{
				"dst": "0.0.0.0/0"
			}
		]
	}
}
`)

	var (
		subnet, _ = types.ParseCIDR("192.168.56.0/24")
		rangeTest = allocator.Range{Subnet: *(*types.IPNet)(subnet)}
		unit      = uint32(4)
		num       = uint32(2 << 3)
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
		rangeTest.Canonicalize()
	})

	AfterEach(func() {
		os.Setenv("ETCD_CFG_DIR", etcdCfgDir)
		os.Setenv("ETCD_ROOT_DIR", etcdRootDir)
		os.Setenv("HOSTNAME", hostname)
	})

	Describe("coveration between lease and ip range", func() {
		It("convert lease to uint32 ip range", func() {
			ip := net.ParseIP("192.168.0.128")
			ipU32 := ipaddr.IP4ToUint32(ip)
			key := filepath.Join("multus", "testtype", "testnet", fmt.Sprintf(rangeTemplate, ipU32, 4))
			ips, ipe := ipamLeaseToUint32Range(key)
			Expect(ips).To(Equal(ipU32))
			Expect(ipe).To(Equal(ipU32 + 16 - 1))
		})

		It("convert lease to simple range", func() {
			ips := net.ParseIP("192.168.0.128")
			expectRS := allocator.SimpleRange{net.ParseIP("192.168.0.128").To4(), net.ParseIP("192.168.0.143").To4()}
			ipsU32 := ipaddr.IP4ToUint32(ips)
			key := filepath.Join("multus", "testtype", "testnet", fmt.Sprintf(rangeTemplate, ipsU32, 4))
			rs := ipamLeaseToSimleRange(key)
			Expect(expectRS.Match(rs)).To(Equal(true))
		})
		It("convert simple range to lease", func() {
			rs := allocator.SimpleRange{net.ParseIP("192.168.0.128"), net.ParseIP("192.168.0.143")}
			ipU32 := ipaddr.IP4ToUint32(rs.RangeStart)
			keyDir := filepath.Join("multus", "testtype", "testnet")
			lease := ipamSimpleRangeToLease(keyDir, &rs)
			Expect(lease).To(Equal("multus/testtype/testnet/" + fmt.Sprintf(rangeTemplate, ipU32, 4)))
		})
	})
	Describe("applying ip from etcd", func() {
		var netConf *allocator.Net
		BeforeEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
			var err error
			netConf, _, err = allocator.LoadIPAMConfig(cniCfg, "")
			if err != nil {
				logging.Debugf("LoadIPAMConfig return %v", err)
			}
		})
		AfterEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
		})

		It("find first ip range", func() {
			em, err := etcdv3.New()
			Expect(err).To(BeNil())
			defer em.Close()
			keyDir := filepath.Join(em.RootKeyDir, leaseDir, "testnet")
			sr, err := ipamGetFreeIPRange(em.Cli, keyDir, &rangeTest, unit)
			Expect(err).To(BeNil())
			Expect(ipaddr.IP4ToUint32(sr.RangeEnd) - ipaddr.IP4ToUint32(sr.RangeStart)).To(Equal(num - 1))

		})

		It("apply first ip range", func() {
			// IpamApplyIPRange is used to apply IP range from ectd
			em, err := etcdv3.New()
			Expect(err).To(BeNil())
			defer em.Close()
			// netConf := allocator.Net{}
			// err = json.Unmarshal(cniCfg, &netConf)
			// Expect(err).To(BeNil())
			Expect(netConf.IPAM.IsFixIP).To(BeFalse())

			sr, err := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
			logging.Debugf("name:%v, range:%v, unit:%v, sr:%v", netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit, sr)
			Expect(err).To(BeNil())
			Expect(ipaddr.IP4ToUint32(sr.RangeEnd) - ipaddr.IP4ToUint32(sr.RangeStart)).To(Equal(num - 1))

			eips, eipe := ipaddr.IP4ToUint32(sr.RangeStart), ipaddr.IP4ToUint32(sr.RangeEnd)

			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)

			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, err := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(1))
			ips, ipe := ipamLeaseToUint32Range(string(resp.Kvs[0].Key))
			Expect(eips).To(Equal(ips))
			Expect(eipe).To(Equal(ipe))
		})
		It("continue apply ip", func() {
			em, err := etcdv3.New()
			Expect(err).To(BeNil())
			defer em.Close()
			// netConf := allocator.Net{}
			// err = json.Unmarshal(cniCfg, &netConf)
			Expect(err).To(BeNil())
			n := 4
			for i := 0; i < n; i++ {
				sr, err := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
				Expect(err).To(BeNil())
				Expect(ipaddr.IP4ToUint32(sr.RangeEnd) - ipaddr.IP4ToUint32(sr.RangeStart)).To(Equal(num - 1))
			}

			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)
			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, err := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(n))
			srs := []allocator.SimpleRange{}
			for _, kv := range resp.Kvs {
				k := string(kv.Key)
				ips, ipe := ipamLeaseToUint32Range(k)
				Expect(ipe - ips).To(Equal(num - 1))
				sr := ipamLeaseToSimleRange(k)
				srs = append(srs, *sr)
			}
			for i1, sr1 := range srs[1:] {
				for i2, sr2 := range srs[:n-1] {
					if i1+1 == i2 {
						continue
					}
					Expect(sr1.Overlaps(&sr2)).To(BeFalse())
				}
			}
		})
		It("interval apply ip", func() {
			em, err := etcdv3.New()
			Expect(err).To(BeNil())
			defer em.Close()
			// netConf := allocator.Net{}
			// err = json.Unmarshal(cniCfg, &netConf)
			Expect(err).To(BeNil())
			n := 3
			var sri *allocator.SimpleRange
			for i := 0; i < n; i++ {
				sr, err := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
				if i == 1 {
					sri = sr
				}
				Expect(err).To(BeNil())
				Expect(ipaddr.IP4ToUint32(sr.RangeEnd) - ipaddr.IP4ToUint32(sr.RangeStart)).To(Equal(num - 1))
			}
			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)
			l := ipamSimpleRangeToLease(keyDir, sri)
			etcdv3.TransDelKey(em.Cli, l)
			sr, err := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
			Expect(err).To(BeNil())
			Expect(sr.Match(sri)).To(BeTrue())
		})
	})
	Describe("verification between etcd and local", func() {
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

		It("etcd have more records than local, after check, local should equal to etcd", func() {
			em, _ := etcdv3.New()
			defer em.Close()
			n := 5
			var srs []*allocator.SimpleRange
			for i := 0; i < n; i++ {
				sr, _ := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
				srs = append(srs, sr)
			}
			s, _ := disk.New(netConf.Name, "")
			caches, _ := s.LoadCache()
			Expect(len(caches)).To(Equal(0))
			s.AppendCache(srs[1])
			s.AppendCache(srs[3])
			caches, _ = s.LoadCache()
			Expect(len(caches)).To(Equal(2))
			IPAMCheckEtcd()
			caches, _ = s.LoadCache()
			Expect(len(caches)).To(Equal(n))
			for _, csr := range caches {
				findMatch := false
				for _, sr := range srs {
					if csr.Match(sr) {
						findMatch = true
						break
					}
				}
				Expect(findMatch).To(BeTrue())
			}
		})
		It("local have more record than etcd, after check, etcd should equal to local", func() {
			em, _ := etcdv3.New()
			defer em.Close()
			// netConf := allocator.Net{}
			// json.Unmarshal(cniCfg, &netConf)
			s, _ := disk.New(netConf.Name, "")
			n := 5
			var srs []*allocator.SimpleRange
			for i := 0; i < n; i++ {
				sr, _ := IPAMApplyIPRange(netConf.Name, &netConf.IPAM.Ranges[0][0], netConf.IPAM.ApplyUnit)
				s.AppendCache(sr)
				srs = append(srs, sr)
			}

			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)

			etcdv3.TransDelKey(em.Cli, ipamSimpleRangeToLease(keyDir, srs[1]))
			etcdv3.TransDelKey(em.Cli, ipamSimpleRangeToLease(keyDir, srs[3]))
			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(n - 2))
			IPAMCheckEtcd()
			ctx, cancel = context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, _ = em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(n))
			for _, ev := range resp.Kvs {
				tmp := ipamLeaseToSimleRange(string(ev.Key))
				findMatch := false
				for _, sr := range srs {
					if sr.Match(tmp) {
						findMatch = true
						break
					}
				}
				Expect(findMatch).To(BeTrue())
			}
		})
		It("etcd record is empty but local have data", func() {
			em, _ := etcdv3.New()
			defer em.Close()
			// netConf := allocator.Net{}
			// json.Unmarshal(cniCfg, &netConf)
			testRS1 := allocator.SimpleRange{net.IPv4(192, 168, 100, 128), net.IPv4(192, 168, 100, 143)}
			testRS2 := allocator.SimpleRange{net.IPv4(192, 168, 100, 160), net.IPv4(192, 168, 100, 175)}
			tests := []*allocator.SimpleRange{&testRS1, &testRS2}
			s, _ := disk.New(netConf.Name, "")
			s.AppendCache(&testRS1)
			s.AppendCache(&testRS2)

			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)
			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(0))
			IPAMCheckEtcd()
			ctx, cancel = context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, _ = em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(2))
			for _, ev := range resp.Kvs {
				tmp := ipamLeaseToSimleRange(string(ev.Key))
				findMatch := false
				for _, sr := range tests {
					if sr.Match(tmp) {
						findMatch = true
						break
					}
				}
				Expect(findMatch).To(BeTrue())
			}
		})
		It("etcd data conflict with local date, local data should be clean", func() {
			em, _ := etcdv3.New()
			defer em.Close()
			// netConf := allocator.Net{}
			// json.Unmarshal(cniCfg, &netConf)
			s, _ := disk.New(netConf.Name, "")
			testRS1 := allocator.SimpleRange{net.IPv4(192, 168, 100, 128), net.IPv4(192, 168, 100, 143)}
			testRS2 := allocator.SimpleRange{net.IPv4(192, 168, 100, 160), net.IPv4(192, 168, 100, 175)}
			tests := []*allocator.SimpleRange{&testRS1, &testRS2}
			s.AppendCache(&testRS1)
			s.AppendCache(&testRS2)

			keyDir := filepath.Join(em.RootKeyDir, leaseDir, netConf.Name)

			for _, rs := range tests {
				em.Cli.Put(context.TODO(), ipamSimpleRangeToLease(keyDir, rs), "nodenoexsit")
			}

			caches, _ := s.LoadCache()
			Expect(len(caches)).To(Equal(2))
			IPAMCheckEtcd()
			caches, _ = s.LoadCache()
			Expect(len(caches)).To(Equal(0))
			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(2))
			for _, ev := range resp.Kvs {
				Expect(string(ev.Value)).To(Equal("nodenoexsit"))
				tmp := ipamLeaseToSimleRange(string(ev.Key))
				findMatch := false
				for _, sr := range tests {
					if sr.Match(tmp) {
						findMatch = true
						break
					}
				}
				Expect(findMatch).To(BeTrue())
			}
		})

	})

	Describe("testing apply fix ip", func() {
		var netConf *allocator.Net
		var namespace = "testns"
		var podName = "testpod"
		BeforeEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
			netConf, _, _ = allocator.LoadIPAMConfig(cniCfg, "")
			netConf.IPAM.IsFixIP = true
		})
		AfterEach(func() {
			em, _ := etcdv3.New()
			defer em.Close()
			em.Cli.Delete(context.TODO(), em.RootKeyDir, clientv3.WithPrefix())
		})

		It("generate and parse fix info", func() {
			fixInfo := IPAMGenFixInfo(namespace, podName, 1)
			Expect(fixInfo).To(Equal(namespace + fixGap + podName + fixGap + "1"))
			parseNS, parsePod := IPAMParseFixInfo(fixInfo)
			Expect(parseNS).To(Equal(namespace))
			Expect(parsePod).To(Equal(podName))
		})

		It("rand apply fix ips and check the ip allocation is fixed", func() {
			em, _ := etcdv3.New()
			defer em.Close()
			// netConf := allocator.Net{}
			// netConf, v, err := allocator.LoadIPAMConfig(cniFixCfg, "")
			// Expect(err).To(BeNil())
			// Expect(netConf.Name).To(Equal("testnet"))
			n := 4
			lease := []*net.IPNet{}
			for i := 0; i < n; i++ {
				pod := podName + strconv.Itoa(i)
				for v := 0; v < n; v++ {
					fixInfo := IPAMGenFixInfo(namespace, pod, v)
					network, err := IPAMApplyFixIP(netConf.Name, netConf.IPAM.FixRange, fixInfo)
					Expect(err).To(BeNil())
					lease = append(lease, network)
				}
			}

			logging.Debugf("lease: %v", lease)

			l := len(lease)

			for i := 0; i < l; i++ {
				for j := 1; j < l; j++ {
					if i == j {
						continue
					}
					Expect(lease[i].String()).NotTo(Equal(lease[j].String()))
				}
				podIndex := int(i / n)
				ifIndex := i % n
				pod := podName + strconv.Itoa(podIndex)
				fixInfo := IPAMGenFixInfo(namespace, pod, ifIndex)
				network, err := IPAMApplyFixIP(netConf.Name, netConf.IPAM.FixRange, fixInfo)
				Expect(err).To(BeNil())
				logging.Debugf("network: info:%v, net:%v", fixInfo, network)
				Expect(lease[i].String()).To(Equal(network.String()))
			}
			ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
			keyDir := filepath.Join(em.RootKeyDir, fixDir, netConf.Name)
			resp, _ := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix())
			cancel()
			Expect(len(resp.Kvs)).To(Equal(n * n))
			for _, ev := range resp.Kvs {
				k := ipaddr.Uint32ToIP4(ipaddr.StrToUint32(filepath.Base(string(ev.Key))))
				match := false
				for _, n := range lease {
					logging.Debugf("%v - %v", n.IP, k)
					if n.IP.String() == k.String() {
						match = true
						break
					}
				}
				Expect(match).To(BeTrue())
			}
		})
	})

})
