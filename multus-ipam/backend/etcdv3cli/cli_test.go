package etcdv3cli

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/intel/multus-cni/ipaddr"
	"github.com/intel/multus-cni/logging"
	"net"
	"fmt"
	"path/filepath"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/containernetworking/plugins/pkg/ip"
)


var _ = Describe("Cli", func() {
	BeforeEach(func() {
		logging.SetLogFile("/tmp/multus-test.log")
		logging.SetLogLevel("debug")
	})
	
    Describe("coveration between lease and ip range",func(){
		It("convert lease to uint32 ip range", func() {
			ip := net.ParseIP("192.168.0.128")
			ipU32 := ipaddr.IP4ToUint32(ip)
			key := filepath.Join("multus","testtype","testnet", fmt.Sprintf(keyTemplate,ipU32,4))
			ips, ipe := ipmaLeaseToUint32Range(key)
			Expect(ips).To(Equal(ipU32))
			Expect(ipe).To(Equal(ipU32+16-1))
		})
	
		It("convert lease to simple range", func() {
			ips := net.ParseIP("192.168.0.128")
			expectRS := allocator.SimpleRange{net.ParseIP("192.168.0.128"),net.ParseIP("192.168.0.143")}
			ipsU32 := ipaddr.IP4ToUint32(ips)
			key := filepath.Join("multus","testtype","testnet", fmt.Sprintf(keyTemplate,ipsU32,4))
			rs := ipamLeaseToSimleRange(key)
			logging.Debugf("%v,%v",expectRS, rs)
			logging.Debugf("%v,%v",ip.Cmp(expectRS.RangeStart, rs.RangeStart), ip.Cmp(expectRS.RangeEnd, rs.RangeEnd))
			Expect(expectRS.Match(rs)).To(Equal(true))
		})
		It("convert simple range to lease", func() {
			rs := allocator.SimpleRange{net.ParseIP("192.168.0.128"),net.ParseIP("192.168.0.143")}
			ipU32 := ipaddr.IP4ToUint32(rs.RangeStart)
			keyDir := filepath.Join("multus","testtype","testnet")
			lease :=ipamSimpleRangeToLease(keyDir, &rs)
			Expect(lease).To(Equal("multus/testtype/testnet/"+fmt.Sprintf(keyTemplate,ipU32,4)))
		})	
	})
})
