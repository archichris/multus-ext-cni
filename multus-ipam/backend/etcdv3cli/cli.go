package etcdv3cli

import (
	"context"
	"math"
	"os"
	"path/filepath"

	"fmt"
	"math/rand"
	"net"

	"strings"

	"github.com/coreos/etcd/clientv3"

	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/ipaddr"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/allocator"
	"github.com/intel/multus-cni/multus-ipam/backend/disk"
)

var (
	leaseDir    = "lease" //multus/netowrkname/key(ipsegment):value(node)
	fixDir      = "fix"
	staticDir   = "static"
	keyTemplate = "%010d-%d"
	maxApplyTry = 3
)

func ipamLeaseToUint32Range(key string) (IPStart uint32, IPEnd uint32) {
	lease := strings.Split(filepath.Base(key), "-")
	IPStart = ipaddr.StrToUint32(lease[0])
	hostSize := ipaddr.StrToUint32(lease[1])
	IPEnd = ipaddr.Uint32AddSeg(IPStart, hostSize) - 1
	return IPStart, IPEnd
}

func ipamLeaseToSimleRange(l string) *allocator.SimpleRange {
	ips, ipe := ipamLeaseToUint32Range(l)
	return &allocator.SimpleRange{ipaddr.Uint32ToIP4(ips), ipaddr.Uint32ToIP4(ipe)}
}

func ipamSimpleRangeToLease(keyDir string, rs *allocator.SimpleRange) string {
	ips := ipaddr.IP4ToUint32(rs.RangeStart)
	n := rs.HostSize()
	return filepath.Join(keyDir, fmt.Sprintf(keyTemplate, ips, n))
}

// IpamApplyIPRange is used to apply IP range from ectd
func IPAMApplyIPRange(network string, r *allocator.Range, unit uint32) (*allocator.SimpleRange, error) {
	logging.Debugf("Going to do apply IP range from %v", *r)
	etcdMultus, err := etcdv3.New()
	if err != nil {
		return nil, err
	}
	cli, rKeyDir, id := etcdMultus.Cli, etcdMultus.RootKeyDir, etcdMultus.Id
	defer cli.Close() // make sure to close the client

	keyDir := filepath.Join(rKeyDir, leaseDir, network)

	dirMutex, err := etcdv3.LockDir(cli, keyDir)
	if err != nil {
		return nil, err
	}
	defer dirMutex.Close()

	rs, err := ipamGetFreeIPRange(cli, keyDir, r, unit)
	if err != nil {
		return nil, err
	}

	logging.Debugf("Going to put %v:%v", ipamSimpleRangeToLease(keyDir, rs), id)

	_, err = cli.Put(context.TODO(), ipamSimpleRangeToLease(keyDir, rs), id)
	if err != nil {
		return nil, logging.Errorf("write key %v to %v failed", ipamSimpleRangeToLease(keyDir, rs), id)
	}

	return rs, nil
}

// GetFreeIPRange is used to find a free IP range
func ipamGetFreeIPRange(cli *clientv3.Client, keyDir string, r *allocator.Range, n uint32) (*allocator.SimpleRange, error) {
	num := uint32(math.Pow(2, float64(n)))
	logging.Debugf("ipamGetFreeIPRange(%v,%v,%v)", keyDir, *r, num)

	rips, ripe := ipaddr.IP4ToUint32(r.RangeStart), ipaddr.IP4ToUint32(r.RangeEnd)
	tmp := ipaddr.IP4ToUint32(r.Subnet.IP) + 2
	if rips < tmp {
		rips = tmp
	}
	last := rips

	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	resp, err := cli.Get(ctx, keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return nil, logging.Errorf("Get %v failed, %v", keyDir, err)
	}
	var sips, sipe uint32
	for _, ev := range resp.Kvs {
		logging.Debugf("Key:%v, Value:%v ", string(ev.Key), string(ev.Value))
		ips, ipe := ipamLeaseToUint32Range(string(ev.Key))
		if ips == 0 {
			logging.Debugf("Invalid Key %v", string(ev.Key))
			continue
		}
		if ips-last < num {
			last = ipe + 1
			continue
		}
		break
	}
	if ripe-last >= num-1 {
		sips = last
		sipe = last + num - 1
		logging.Debugf("get IP range (%v-%v) from (%v-%v)", sips, sipe, rips, ripe)
		return &allocator.SimpleRange{ipaddr.Uint32ToIP4(sips), ipaddr.Uint32ToIP4(sipe)}, nil
	}
	return nil, logging.Errorf("apply ip range failed")
}

func IPAMGetAllLease(cli *clientv3.Client, keyDir, id string) (map[string][]allocator.SimpleRange, error) {
	logging.Debugf("Going to get all IP lease belong to %v from %v", id, keyDir)
	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	resp, err := cli.Get(ctx, keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		return nil, logging.Errorf("Get %v failed, %v", keyDir, err)
	}
	leases := make(map[string][]allocator.SimpleRange)
	for _, ev := range resp.Kvs {
		v := strings.Trim(string(ev.Value), " \r\n\t")
		logging.Debugf("Key:%v, Value:%v, id:%v, match:%v ", string(ev.Key), v, id, v == id)
		if v == id {
			k := strings.Trim(string(ev.Key), " \r\n\t")
			network := filepath.Base(filepath.Dir(k))
			sr := ipamLeaseToSimleRange(k)
			if _, ok := leases[network]; ok {
				leases[network] = append(leases[network], *sr)
			} else {
				leases[network] = []allocator.SimpleRange{*sr}
			}
		}
	}
	return leases, nil
}

func ipamCheckNet(em *etcdv3.EtcdMultus, network string, leases []allocator.SimpleRange) {

	s, err := disk.New(network, "")
	if err != nil {
		logging.Errorf("create disk manager failed, %v", err)
		return
	}
	caches, err := s.LoadCache()
	if err != nil {
		logging.Errorf("get cache failed, %v", err)
		return
	}
	logging.Debugf("check net:%v\nleases:%v\ncaches:%v\n", network, leases, caches)
	keyDir := filepath.Join(em.RootKeyDir, leaseDir, network)
	cli, id := em.Cli, em.Id
	var last *allocator.SimpleRange
	for _, lsr := range leases {
		last = nil
		for _, csr := range caches {
			if csr.Overlaps(&lsr) {
				if csr.Match(&lsr) {
					last = &csr
					break
				} else {
					// caches = delete(caches, csr)
					s.DeleteCache(&csr)
				}
			}
		}
		if last == nil {
			err := s.AppendCache(&lsr)
			if err != nil {
				etcdv3.TransDelKey(cli, ipamSimpleRangeToLease(keyDir, &lsr))
			}
		}
	}

	caches, err = s.LoadCache()
	if err != nil {
		logging.Errorf("get cache failed, %v", err)
		return
	}
	for _, csr := range caches {
		last = nil
		var lsr allocator.SimpleRange
		for _, lsr = range leases {
			if csr.Match(&lsr) {
				last = &csr
				break
			}
		}
		logging.Debugf("cache:%v, lease:%v, result:%v", csr, lsr, last)
		if last == nil {
			err = etcdv3.TransPutKey(cli, ipamSimpleRangeToLease(keyDir, &csr), id, true)
			if err != nil {
				logging.Debugf("going to delete error cache:%v", csr)
				s.DeleteCache(&csr)
			}
		}
	}
}

func IPAMCheck() error {
	logging.Debugf("Going to check IPAM")
	etcdMultus, err := etcdv3.New()
	cli, rKeyDir, id := etcdMultus.Cli, etcdMultus.RootKeyDir, etcdMultus.Id
	if err != nil {
		return err
	}
	defer cli.Close() // make sure to close the client

	lDir := filepath.Join(rKeyDir, leaseDir)

	leases, err := IPAMGetAllLease(cli, lDir, id)
	if err != nil {
		return err
	}

	localNets := disk.GetAllNet(os.Getenv("NET_DATA_DIR"))
	logging.Debugf("local net: %v", localNets)

	for network, lease := range leases {
		ipamCheckNet(etcdMultus, network, lease)
		for idx, n := range localNets {
			if network == n {
				if idx == 0 {
					localNets = localNets[1:]
				} else if idx == len(localNets)-1 {
					localNets = localNets[:len(localNets)-1]
				} else {
					localNets = append(localNets[:idx], localNets[idx+1:]...)
				}
				break
			}
		}
	}

	for _, network := range localNets {
		ipamCheckNet(etcdMultus, network, nil)
	}

	return nil
}

// GetFreeIPRange is used to find a free IP range
func IPAMApplyFixIP(network string, r *allocator.Range, fixInfo string) (*net.IPNet, error) {
	// netConf *allocator.Net
	logging.Debugf("Going to do apply fix IP from %v", r)
	em, err := etcdv3.New()
	if err != nil {
		return nil, err
	}
	// cli, rKeyDir, id := etcdMultus.Cli, etcdMultus.RootKeyDir, etcdMultus.Id
	defer em.Close() // make sure to close the client

	keyDir := filepath.Join(em.RootKeyDir, fixDir, network)

	dirMutex, err := etcdv3.LockDir(em.Cli, keyDir)
	if err != nil {
		return nil, err
	}
	defer dirMutex.Close()

	ctx, cancel := context.WithTimeout(context.Background(), etcdv3.RequestTimeout)
	resp, err := em.Cli.Get(ctx, keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	freeIPs := []uint32{}
	fixIP := uint32(0)
	rips, ripe := ipaddr.IP4ToUint32(r.RangeStart), ipaddr.IP4ToUint32(r.RangeEnd)
	tmp := ipaddr.IP4ToUint32(r.Subnet.IP) + 2
	if rips < tmp {
		rips = tmp
	}
	last := rips
	for _, ev := range resp.Kvs {
		// logging.Debugf("Key:%v, Value:%v ", string(ev.Key), string(ev.Value))

		ip := ipaddr.StrToUint32(filepath.Base(string(ev.Key)))
		if string(ev.Value) == fixInfo {
			fixIP = ip
		}

		if ip-last > 0 {
			for i := last; i < ip; i++ {
				freeIPs = append(freeIPs, i)
			}
		}

		last = ip + 1
	}

	if fixIP == 0 {
		for i := last; i < ripe+1; i++ {
			freeIPs = append(freeIPs, i)
		}
		if len(freeIPs) > 0 {
			fixIP = freeIPs[rand.Intn(len(freeIPs))]
		} else {
			return nil, logging.Errorf("no availble fixed ip")
		}
	}

	key := filepath.Join(keyDir, fmt.Sprintf("%010d", fixIP))

	logging.Debugf("Going to put %v:%v", key, fixInfo)

	_, err = em.Cli.Put(context.TODO(), key, fixInfo)
	if err != nil {
		return nil, logging.Errorf("write key %v to %v failed", key, fixInfo)
	}
	return &net.IPNet{IP: ipaddr.Uint32ToIP4(fixIP), Mask: r.Subnet.Mask}, nil
}
