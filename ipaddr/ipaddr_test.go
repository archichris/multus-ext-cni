package ipaddr_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/intel/multus-cni/ipaddr"
	"net"
)

var _ = Describe("Ipaddr", func() {
	It("convert IP to uint32", func() {
		testIP := net.ParseIP("192.168.100.1")
		value := uint32(192 << 24 + 168 << 16 + 100 <<8 + 1)
		result :=  IP4ToUint32(testIP)
		Expect(result).To(Equal(value))
	})
	It("convert uint32 to IP", func() {
		testIP := net.ParseIP("192.168.100.1")
		value := uint32(192 << 24 + 168 << 16 + 100 <<8 + 1)
		result :=  Uint32ToIP4(value).String()
		Expect(result).To(Equal(testIP.String()))
	})

	It("ip plus a segment", func() {
		testIP := IP4ToUint32(net.ParseIP("192.168.100.1"))
		targetIP := IP4ToUint32(net.ParseIP("192.168.101.1"))
		result := Uint32AddSeg(testIP, 8)
		Expect(targetIP).To(Equal(result))
	})

	It("convert net to uint32 start ip and uint32 end ip", func() {
		_,testNet,_ := net.ParseCIDR("192.168.100.0/24")
		ipStart := uint32(192 << 24 + 168 << 16 + 100 <<8)
		ipEnd := uint32(192 << 24 + 168 << 16 + 101 <<8 - 1)  
		resultS, resultE :=  Net4To2Uint32(testNet)
		Expect(ipStart).To(Equal(resultS))
		Expect(ipEnd).To(Equal(resultE))
	})
})
