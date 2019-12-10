package ipaddr

import (
	"math/big"
	"net"
	"strconv"
)

func IP4ToUint32(ip net.IP) uint32 {
	if v := ip.To4(); v != nil {
		return uint32(big.NewInt(0).SetBytes(v).Uint64())
	} else {
		return 0
	}
}

func Uint32ToIP4(i uint32) net.IP {
	return net.IP(big.NewInt(0).SetUint64(uint64(i)).Bytes())
}

func Uint32AddSeg(ip uint32, s uint32) uint32 {
	return ip + uint32(2<<(s-1))
}

func Net4To2Uint32(n *net.IPNet) (uint32, uint32) {
	ipStart := IP4ToUint32(n.IP.Mask(n.Mask))
	ones, bits := n.Mask.Size()
	ipEnd := Uint32AddSeg(ipStart, uint32(bits-ones)) - 1
	return ipStart, ipEnd
}

func StrToUint32(s string) uint32 {
	tmpU64, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(tmpU64)
}
