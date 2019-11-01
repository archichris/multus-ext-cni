package disk

import (
	"net"
	"os"
	"path/filepath"

	"github.com/containernetworking/plugins/pkg/ip"
	// . "github.com/intel/multus-cni/backend/disk"
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
	})
	AfterEach(func() {
		os.RemoveAll(filepath.Join(dataDir, network))
	})

	It("should return zero IP when gateway file does not exist", func() {
		store, _ := New(network, dataDir)
		gw := store.LoadGW("gateway", "gateway")
		Expect(ip.Cmp(gw, net.IPv4zero)).To(Equal(0))
	})

	It("should get and clear gateway IP correctly", func() {
		store, _ := New(network, dataDir)
		store.Reserve("gateway", "gateway", net.ParseIP(gwIP), "0")
		gw := store.LoadGW("gateway", "gateway")
		Expect(ip.Cmp(gw, net.IPv4zero)).NotTo(Equal(0))
		Expect(ip.Cmp(gw, net.ParseIP(gwIP))).To(Equal(0))
		lgw := store.GetByID("gateway", "gateway")[0]
		Expect(ip.Cmp(gw, lgw)).To(Equal(0))
		store.ReleaseByID("gateway", "gateway")
		gw = store.LoadGW("gateway", "gateway")
		Expect(ip.Cmp(gw, net.IPv4zero)).To(Equal(0))
	})
})
