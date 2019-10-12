package etcdv3

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/intel/multus-cni/logging"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/pkg/transport"
)

var (
	dialTimeout       = 5 * time.Second
	requestTimeout    = 5 * time.Second
	defaultEtcdCfgDir = "/etc/cni/net.d/multus.d/etcd"
	rootKeyDir        = "multus" //multus/netowrkname/key(ipsegment):value(node)
	keyTemplate       = "%10d-%10d"
	maxApplyTry       = 3
)

// EtcdJSONCfg is the struct of stored etcd information
type EtcdJSONCfg struct {
	Name      string `json:"name"`
	Endpoints string `json:"endpoints"`
	// Namespace string `json:"namespace"`
	// ClientPort string  `json:"clientPort"`
	// Replicas   int     `json:"replicas`
	Auth AuthCfg `json:"auth"`
}

type AuthCfg struct {
	Client AuthClient `json:"client"`
	Peer   AuthPeer   `json:"peer"`
}

type AuthClient struct {
	SecureTransport      bool   `json:"secureTransport"`
	EnableAuthentication bool   `json:"enableAuthentication"`
	SecretDirectory      string `json:"secretDirectory"`
}

type AuthPeer struct {
	SecureTransport      bool `json:"secureTransport"`
	EnableAuthentication bool `json:"enableAuthentication"`
	UseAutoTLS           bool `json:"useAutoTLS"`
}

// ApplyNewIPRange is used to apply IP range from ectd
func ApplyNewIPRange(network string, subnet *types.IPNet, unit uint32) (net.IP, net.IP, error) {

	etcdCfgDir := os.Getenv("ETCD_CFG_DIR")
	if etcdCfgDir == "" {
		etcdCfgDir = defaultEtcdCfgDir
	}
	id := os.Getenv("HOSTNAME")
	if id == "" {
		data, err := ioutil.ReadFile(etcdCfgDir + "/id")
		if err == nil {
			id = string(data)
		} else {
			return nil, nil, logging.Errorf("empty hostname")
			// only for test
			// rand.Seed(10000)
			// id = strconv.Itoa(rand.Intn(10000))
			// ioutil.WriteFile(etcdCfgDir+"/id", []byte(id), 0644)
		}
	}

	data, err := ioutil.ReadFile(etcdCfgDir + "/etcd.conf")
	if err != nil {
		return nil, nil, logging.Errorf("read %v/etcd.conf failed, %v", etcdCfgDir, err)
	}
	var etcdCfg EtcdJSONCfg
	err = json.Unmarshal(data, &etcdCfg)
	if err != nil {
		return nil, nil, err
	}

	endpoints := strings.Split(etcdCfg.Endpoints, ",")

	if len(endpoints) == 0 {
		return nil, nil, logging.Errorf("no etcd endpoints")
	}

	var cli *clientv3.Client

	if etcdCfg.Auth.Client.SecureTransport {
		tlsInfo := transport.TLSInfo{
			CertFile:      etcdCfg.Auth.Client.SecretDirectory + "/etcd-client.crt",
			KeyFile:       etcdCfg.Auth.Client.SecretDirectory + "/etcd-client.key",
			TrustedCAFile: etcdCfg.Auth.Client.SecretDirectory + "/etcd-client-ca.crt",
		}
		tlsConfig, err := tlsInfo.ClientConfig()
		if err != nil {
			return nil, nil, logging.Errorf("tlsInfo.ClientConfig failed, %v", err)
		}
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: dialTimeout,
			TLS:         tlsConfig,
		})
		if err != nil {
			return nil, nil, logging.Errorf("create etcd client failed, %v", err)
		}
	} else {
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: dialTimeout,
		})
		if err != nil {
			return nil, nil, logging.Errorf("create etcd client failed, %v", err)
		}
	}

	defer cli.Close() // make sure to close the client

	keyDir := rootKeyDir + "/" + network

	// Get free IP range looply
	var lastErr error
	for i := 1; i <= maxApplyTry; i++ {
		IPBegin, IPEnd, err := GetFreeIPRange(cli, keyDir, subnet, unit)
		if err != nil {
			lastErr = logging.Errorf("create etcd client failed, %v", err)
			continue
		}
		claimKey := fmt.Sprintf(keyTemplate, IPBegin, IPEnd)

		getResp, err := cli.Get(context.TODO(), keyDir+"/"+claimKey)
		if len(getResp.Kvs) > 0 {
			lastErr = logging.Errorf("%v/%v exist", keyDir, claimKey)
			continue
		}

		// Claim the ownship of the IP range
		_, err = cli.Put(context.TODO(), keyDir+"/"+claimKey, id)
		if err != nil {
			lastErr = logging.Errorf("write etcd failed, %v", err)
			continue
		}
		// Verify the ownship of the IP range

		for i := 1; i <= maxApplyTry; i++ {
			getResp, err = cli.Get(context.TODO(), keyDir+"/"+claimKey)
			if (err != nil) || (len(getResp.Kvs) == 0) {
				lastErr = logging.Errorf("read etcd failed, %v", err)
				time.Sleep(time.Duration(1) * time.Second)
				continue
			}
		}
		if (err != nil) || (len(getResp.Kvs) == 0) {
			return nil, nil, logging.Errorf("Operate etcd failed, %v", err)
		}
		// if putResp.Header.Revision == getResp.Header.Revision {
		if string(getResp.Kvs[0].Value) == id {
			beginIP := make(net.IP, 4)
			endIP := make(net.IP, 4)
			binary.BigEndian.PutUint32(beginIP, IPBegin)
			binary.BigEndian.PutUint32(endIP, IPEnd)
			return beginIP, endIP, nil
		}
	}
	return nil, nil, lastErr
}

// GetFreeIPRange is used to find a free IP range
func GetFreeIPRange(cli *clientv3.Client, keyDir string, subnet *types.IPNet, unit uint32) (uint32, uint32, error) {
	bIP := binary.BigEndian.Uint32(subnet.IP.To4()) + 1
	eIP := bIP + ^binary.BigEndian.Uint32(subnet.Mask) - 1
	lastIP := bIP
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	getResp, err := cli.Get(ctx, keyDir, clientv3.WithPrefix(), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	cancel()
	if err != nil {
		logging.Errorf("cli.Get(%v) failed, %v", keyDir, err)
		return 0, 0, err
	}
	var IPBegin, IPEnd uint32
	for _, ev := range getResp.Kvs {
		IPRange := strings.Split(string(ev.Key)[strings.LastIndex(string(ev.Key), "/")+1:], "-")
		tmpU64, _ := strconv.ParseUint(IPRange[0], 10, 32)
		IPRangeBegin := uint32(tmpU64)
		if IPRangeBegin-lastIP <= 1 {
			tmpU64, _ = strconv.ParseUint(IPRange[1], 10, 32)
			lastIP = uint32(tmpU64)
			continue
		}
		IPBegin = lastIP + 1
		IPEnd = IPRangeBegin - 1
		return IPBegin, IPEnd, nil
	}

	if lastIP < eIP {
		block := eIP - lastIP
		if block > unit {
			block = unit
		}
		IPBegin = lastIP + 1
		IPEnd = lastIP + block
		return IPBegin, IPEnd, nil
	}
	return 0, 0, errors.New("can't apply free IP range")
}

func main() {
	// endpoints := []string{"10.96.232.136:6666"}
	unit := uint32(16)
	// nodeName := "node201"
	_, n, _ := net.ParseCIDR("192.168.56.0/24")
	sIP, eIP, err := ApplyNewIPRange("mac-vlan-1", (*types.IPNet)(n), unit)
	if err == nil {
		fmt.Println(sIP.String() + ":" + eIP.String())
	} else {
		fmt.Println(err)
	}
}
