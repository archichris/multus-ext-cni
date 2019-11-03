package dockercli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/intel/multus-cni/multus-ipam/backend/disk"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cli", func() {
	var dataDir = "/tmp"
	var network = "testnet"
	BeforeEach(func() {
		os.RemoveAll(filepath.Join(dataDir, network))
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
	})
	AfterEach(func() {
		// os.RemoveAll(filepath.Join(dataDir, network))
	})

	It("check process clear all ips with ip not exist", func() {
		store, _ := disk.New(network, dataDir)
		idTmp := "testid%d"
		ifname := "eth1"
		startIP := net.IPv4(192, 168, 200, 100)
		// gwIP := net.IPv4(192, 168, 200, 1)
		store.Reserve("gateway", "gateway", startIP, "0")
		curIP := ip.NextIP(startIP)
		for i := 0; i < 5; i++ {
			store.Reserve(fmt.Sprintf(idTmp, i), ifname, curIP, "0")
			curIP = ip.NextIP(curIP)
		}
		store.AppendCache(&allocator.SimpleRange{startIP, curIP})

		leases := disk.LoadAllLeases(network, dataDir)
		Expect(len(leases)).To(Equal(6))
		logging.Debugf("leases: %v", leases)
		IPAMCheckLocalIPs(dataDir)
		leases = disk.LoadAllLeases(network, dataDir)
		Expect(len(leases)).To(Equal(1))
		gw := filepath.Join(store.Dir(), startIP.String())
		Expect(leases[gw]).To(Equal("gateway"))
	})

})
