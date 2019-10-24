package etcdv3

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"path/filepath"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/intel/multus-cni/logging"
)

var (
	dialTimeout       = 5 * time.Second
	defaultEtcdCfgDir = "/etc/cni/net.d/multus.d/etcd"
	RequestTimeout    = 5 * time.Second
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

func KeyToMutex(key string) string {
	root := filepath.Dir(key)
	kind := filepath.Base(root)
	root = filepath.Dir(root)
	return filepath.Join(root, "mutex", kind)
}

func TransDelKeys(c *clientv3.Client, keys []string) error {
	cli := c
	if cli == nil {
		cli, _, err := NewClient()
		if err != nil {
			return logging.Errorf("Create etcd client failed, %v", err)
		}
		defer cli.Close()
	}

	s, err := concurrency.NewSession(cli)
	if err != nil {
		return logging.Errorf("create etcd session failed, %v", err)
	}
	defer s.Close()

	var mutex string = ""
	var m *concurrency.Mutex
	logging.Debugf("going to del %v from etcd", keys)
	for _, key := range keys {
		tmp := KeyToMutex(key)
		logging.Debugf("old mutex:%v, new mutex:%v, m:%v", mutex, tmp, m)
		if mutex != tmp {
			if m != nil {
				err = m.Unlock(context.TODO())
				if err != nil {
					logging.Errorf("unlock %v failed, %v", mutex, err)
				}
			}
			mutex = tmp
			m = concurrency.NewMutex(s, mutex)
			if err := m.Lock(context.TODO()); err != nil {
				logging.Errorf("lock %v failed, %v", mutex, err)
				mutex = ""
				m = nil
				continue
			}
		}
		_, err = cli.Delete(context.TODO(), key)
		if err != nil {
			logging.Errorf("del key %v to %v failed", key)
		}
		logging.Debugf("Del key %v", key)
	}

	if m != nil {
		if m.Unlock(context.TODO()); err != nil {
			logging.Errorf("lock %v failed, %v", mutex, err)
		}
	}
	return nil
}
