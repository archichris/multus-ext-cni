package vxlan

import (
	"fmt"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// IPToMac(host net.IP, guest net.IP) net.HardwareAddr{
// 	return (net.HardwareAddr)(
// 		append(
// 			privateMACPrefix,
// 			host.To4()[2:4]..., guest.To4()[2:4]),
// 	), nil
// }

// var (
// 	hostIPTmp  = "10.100.%d.%d"
// 	guestIPTmp = "192.168.%d.%d"
// 	lladdrTmp  = "fe:ff:%x:%x:%x:%x"
// 	privateMACPrefix = []byte{0xa, 0xa}
// )
var _ = Describe("Bridge", func() {
	hostIP := net.IPv4(10, 100, 100, 100)
	guestIP := net.IPv4(192, 168, 56, 31)
	expectMac := fmt.Sprintf("%s:64:64:38:1f", macPrefixStr)
	It("IP to Mac", func() {
		mac := IPToMac(hostIP, guestIP)
		Expect(mac.String()).To(Equal(expectMac))
	})
	It("Mac to IP", func() {
		mac, _ := net.ParseMAC(expectMac)
		host, guest := MacToIP(mac)
		Expect(host.String()).To(Equal(hostIP.String()))
		Expect(guest.String()).To(Equal(guestIP.String()))
	})
})
