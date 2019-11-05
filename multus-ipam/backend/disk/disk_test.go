package disk

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Disk", func() {
	var (
		dataDir = "/tmp"
		network = "testnet"
		gwIP    = "192.168.200.1"
		// ifName  = "eth1"
	)

	BeforeEach(func() {
		os.RemoveAll(filepath.Join(dataDir, network))
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
	})
	AfterEach(func() {
		os.RemoveAll(filepath.Join(dataDir, network))
	})

	It("should return zero IP when gateway file does not exist", func() {
		store, _ := New(network, dataDir)
		gws := store.GetByID("gateway", "gateway")
		Expect(gws).To(BeNil())
	})

	It("should get and clear gateway IP correctly", func() {
		store, _ := New(network, dataDir)
		store.Reserve("gateway", "gateway", net.ParseIP(gwIP), "0")
		gws := store.GetByID("gateway", "gateway")
		Expect(gws).NotTo(BeNil())
		Expect(ip.Cmp(gws[0], net.ParseIP(gwIP))).To(Equal(0))
		lgw := store.GetByID("gateway", "gateway")[0]
		Expect(ip.Cmp(gws[0], lgw)).To(Equal(0))
		store.ReleaseByID("gateway", "gateway")
		gws = store.GetByID("gateway", "gateway")
		Expect(gws).To(BeNil())
	})

	It("get id from file correctly", func() {
		store, _ := New(network, dataDir)
		store.Reserve("gateway", "gateway", net.ParseIP(gwIP), "0")
		Expect(GetID(filepath.Join(dataDir, network, gwIP))).To(Equal("gateway"))
		id := "dlkfangmafodfadfdjgamds1223fef"
		testIP := net.IPv4(192, 168, 200, 100)
		store.Reserve("dlkfangmafodfadfdjgamds1223fef", "eth1", testIP, "0")
		Expect(GetID(filepath.Join(dataDir, network, testIP.String()))).To(Equal(id))
	})

	It("get all leases from data dir", func() {
		testNets := []string{"testnet1", "testnet2"}
		testIPs := []net.IP{net.IPv4(10, 0, 101, 100), net.IPv4(10, 0, 102, 100)}
		for idx, tn := range testNets {
			store, _ := New(tn, dataDir)
			store.Reserve("gateway", "gateway", testIPs[idx], "0")
			curIP := ip.NextIP(testIPs[idx])
			for i := 0; i < 5; i++ {
				store.Reserve(fmt.Sprintf("%s%d", tn, i), fmt.Sprintf("eth%d", idx), curIP, "0")
				curIP = ip.NextIP(curIP)
			}
			store.AppendCache(&allocator.SimpleRange{testIPs[idx], curIP})
		}

		leases := LoadAllLeases("", dataDir)
		Expect(len(leases)).To(Equal(12))
		for file, id := range leases {
			network := filepath.Base(filepath.Dir(file))
			match := false
			for _, n := range testNets {
				if n == network {
					match = true
					break
				}
			}
			Expect(match).To(BeTrue())
			match = false
			for i := 0; i < 5; i++ {
				if id == fmt.Sprintf("%s%d", network, i) || id == "gateway" {
					match = true
					break
				}
			}
			Expect(match).To(BeTrue())
		}
	})
})
