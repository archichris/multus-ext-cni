package etcdv3

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/intel/multus-cni/logging"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
)

var (
	dialTimeout       = 5 * time.Second
	defaultEtcdCfgDir = "/etc/cni/net.d/multus.d/etcd"
)

// etcdCfg is the struct of stored etcd information
type etcdCfg struct {
	Name      string `json:"name"`
	Endpoints string `json:"endpoints"`
	// Namespace string `json:"namespace"`
	// ClientPort string  `json:"clientPort"`
	// Replicas   int     `json:"replicas`
	Auth authCfg `json:"auth"`
}

type authCfg struct {
	Client authClient `json:"client"`
	Peer   authPeer   `json:"peer"`
}

type authClient struct {
	SecureTransport      bool   `json:"secureTransport"`
	EnableAuthentication bool   `json:"enableAuthentication"`
	SecretDirectory      string `json:"secretDirectory"`
}

type authPeer struct {
	SecureTransport      bool `json:"secureTransport"`
	EnableAuthentication bool `json:"enableAuthentication"`
	UseAutoTLS           bool `json:"useAutoTLS"`
}

//NewClient Create a new etcd client, and provide a unify id  for node
func NewClient() (*clientv3.Client, string, error) {
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
			return nil, "", logging.Errorf("can not get id from " + etcdCfgDir + "/id")
		}
	}

	data, err := ioutil.ReadFile(etcdCfgDir + "/etcd.conf")
	if err != nil {
		log.Println(err)
		return nil, "", logging.Errorf("can not get etcd config from " + etcdCfgDir + "/etcd.conf")
	}
	var etcdCfg etcdCfg
	err = json.Unmarshal(data, &etcdCfg)
	if err != nil {
		return nil, "", logging.Errorf("etcd config is not right, %v", err)
	}

	endpoints := strings.Split(etcdCfg.Endpoints, ",")

	if len(endpoints) == 0 {
		return nil, "", logging.Errorf("no etcd endpoints")
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
			return nil, "", logging.Errorf("create tls config failed, %v", err)
		}
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: dialTimeout,
			TLS:         tlsConfig,
		})
		if err != nil {
			return nil, "", logging.Errorf("create etcd client failed, %v", err)
		}
	} else {
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: dialTimeout,
		})
		if err != nil {
			log.Println(err)
			return nil, "", logging.Errorf("create etcd client failed, %v", err)
		}
	}
	return cli, id, nil
}
