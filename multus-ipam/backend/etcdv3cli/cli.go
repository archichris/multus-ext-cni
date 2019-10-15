package etcdv3cli

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"time"

	"fmt"
	"net"

	"strconv"
	"strings"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/intel/multus-cni/dev"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
)

var (
	rootKeyDir     = "multus" //multus/netowrkname/key(ipsegment):value(node)
	keyTemplate    = "%010d-%d-%s"
	requestTimeout = 5 * time.Second
	maxApplyTry    = 3
)

type IfInfo struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	MAC  string `json:"mac"`
}

type LeaseV struct {
	M     *IfInfo `json:"master"`
	Vxlan *IfInfo `json:"vxlan"`
}

type LeaseK struct {
	N  net.IPNet
	Id string
}

type Lease struct {
	K *LeaseK
	V *LeaseV
}

// IpamApplyIPRange is used to apply IP range from ectd
func IpamApplyIPRange(netConf *allocator.Net, subnet *types.IPNet) (net.IP, net.IP, error) {
	value, err := IpamFormValue(netConf)
	if err != nil {
		return nil, nil, err
	}
	logging.Debugf("value for etcd: %s", value)

	IPStart, IPEnd, err := ipamApplyIPRangeUint32(netConf.Type+"/"+netConf.Name, subnet, netConf.IPAM.ApplyUnit, value)
	if err == nil {
		IPs := make(net.IP, 4)
		IPe := make(net.IP, 4)
		binary.BigEndian.PutUint32(IPs, IPStart)
		binary.BigEndian.PutUint32(IPe, IPEnd)
		logging.Debugf("get IP range (%v-%v) from (%v-%v)", IPs, IPe, IPStart, IPEnd)
		return IPs, IPe, nil
	}
	return nil, nil, err
}

func ipamApplyIPRangeUint32(network string, subnet *types.IPNet, n uint32, value string) (uint32, uint32, error) {
	logging.Debugf("ipamApplyIPRangeUint32(%v,%v,%v,%v)", network, *subnet, n, value)
	cli, id, err := etcdv3.NewClient()
	if err != nil {
		return 0, 0, err
	}
	defer cli.Close() // make sure to close the client

	s, err := concurrency.NewSession(cli)
	if err != nil {
		return 0, 0, logging.Errorf("create etcd session failed, %v", err)
	}
	defer s.Close()

	keyDir := rootKeyDir + "/" + network + "/lease"
	keyMutex := rootKeyDir + "/mutex/" + network + "/lease"

	m := concurrency.NewMutex(s, keyMutex)

	// acquire lock for s
	if err := m.Lock(context.TODO()); err != nil {
		return 0, 0, logging.Errorf("get etcd locd failed, %v", err)
	}

	defer func() {
		if err := m.Unlock(context.TODO()); err != nil {
			logging.Debugf("unlock etcd mutex failed, %v", err)
		}
	}()

	IPBegin, IPEnd, err := ipamGetFreeIPRange(cli, keyDir, subnet, n)
	if err != nil {
		return 0, 0, err
	}

	claimKey := fmt.Sprintf(keyTemplate, IPBegin, n, strings.Trim(id, "-:\n\t "))
	_, err = cli.Put(context.TODO(), keyDir+"/"+claimKey, value)
	if err != nil {
		return 0, 0, logging.Errorf("write key %v to %v failed", keyDir+"/"+claimKey, value)
	}
	return IPBegin, IPEnd, nil
}

// GetFreeIPRange is used to find a free IP range
func ipamGetFreeIPRange(cli *clientv3.Client, keyDir string, subnet *types.IPNet, n uint32) (uint32, uint32, error) {
	unit := uint32(math.Pow(2, float64(n)))
	logging.Debugf("ipamGetFreeIPRange(%v,%v,%v)", keyDir, *subnet, unit)
	bIP := binary.BigEndian.Uint32(subnet.IP.To4())
	eIP := bIP + ^binary.BigEndian.Uint32(subnet.Mask)
	lastIP := bIP
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	getResp, err := cli.Get(ctx, keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return 0, 0, logging.Errorf("Get %v failed, %v", keyDir, err)
	}
	var IPBegin, IPEnd uint32
	for _, ev := range getResp.Kvs {
		logging.Debugf("Key:%v, Value:%v ", string(ev.Key), string(ev.Value))
		IPLease := strings.Split(string(ev.Key)[strings.LastIndex(string(ev.Key), "/")+1:], "-")
		tmpU64, _ := strconv.ParseUint(IPLease[0], 10, 32)
		IPRangeBegin := uint32(tmpU64)
		if IPRangeBegin-lastIP < unit {
			tmpU64, _ = strconv.ParseUint(IPLease[1], 10, 32)
			lastIP = IPRangeBegin + uint32(math.Pow(2, float64(tmpU64)))
			continue
		}
		IPBegin = lastIP
		IPEnd = lastIP + unit - 1
		logging.Debugf("get IP range (%v-%v) from (%v-%v) mode 1", IPBegin, IPEnd, bIP, eIP)
		return IPBegin, IPEnd, nil
	}

	if eIP-lastIP+1 >= unit {
		IPBegin = lastIP
		IPEnd = lastIP + unit
		logging.Debugf("get IP range (%v-%v) from (%v-%v) mode 2", IPBegin, IPEnd, bIP, eIP)
		return IPBegin, IPEnd, nil
	}
	return 0, 0, logging.Errorf("There is no available IP")
}

func IpamGenIfInfo(ifName string) *IfInfo {
	i := IfInfo{IP: string("0.0.0.0"), MAC: string("00:00:00:00:00:00"), Name: ""}
	if len(ifName) == 0 {
		logging.Errorf("empty interface name")
		return &i
	}
	i.Name = ifName
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		logging.Verbosef("get interface %s failed, %s", ifName, err)
		return &i
	}
	i.MAC = iface.HardwareAddr.String()
	ifaceAddr, err := dev.GetIfaceIP4Addr(iface)
	if err != nil {
		logging.Verbosef("GetIfaceIP4Addr %s failed, %s", ifName, err)
		return &i
	}
	i.IP = ifaceAddr.String()
	return &i
}

func IpamFormValue(netConf *allocator.Net) (string, error) {
	kv := &LeaseV{M: IpamGenIfInfo(netConf.Master)}

	if netConf.Type == "multus-vxlan" {
		vx := fmt.Sprintf("multus.%v.%v", netConf.Master, netConf.Vxlan.VxlanId)
		logging.Debugf("Try to get info of vxlan %v", vx)
		kv.Vxlan = IpamGenIfInfo(vx)
	}
	logging.Debugf("Type:%v,%v", netConf.Type, kv)

	value, err := json.Marshal(kv)
	if err != nil {
		return "", err
	}
	return string(value), err
}

func IpamGetLeaseInfo(key, value []byte) (*Lease, error) {
	IPLease := strings.Split(string(key)[strings.LastIndex(string(key), "/")+1:], "-")
	ip := make(net.IP, 4)
	tmpU64, _ := strconv.ParseUint(IPLease[0], 10, 32)
	binary.BigEndian.PutUint32(ip, uint32(tmpU64))
	tmpU64, _ = strconv.ParseUint(IPLease[1], 10, 32)
	n := net.IPNet{Mask: net.CIDRMask(32-int(tmpU64), 32), IP: ip}
	k := LeaseK{N: n, Id: string(IPLease[2])}
	v := LeaseV{}
	err := json.Unmarshal(value, &v)
	if err != nil {
		return nil, logging.Errorf("parse value %v failed, %v", value, err)
	}
	return &Lease{K: &k, V: &v}, nil
}
