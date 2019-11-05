package etcdv3cli

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/intel/multus-cni/disk"
	"github.com/intel/multus-cni/etcdv3"
	"github.com/intel/multus-cni/logging"
	"github.com/vishvananda/netlink"
)

var (
	vxlanKeyDir = "vxlan"
	cacheDir    = "/var/lib/cni/mulvx"
)

func RecVxlan(network string, vxlan *netlink.Vxlan) error {
	em, err := etcdv3.New()
	if err != nil {
		return err
	}
	defer em.Close() // make sure to close the client

	key := filepath.Join(em.RootKeyDir, vxlanKeyDir, vxlan.Attrs().Name, vxlan.SrcAddr.String())

	err = etcdv3.TransPutKey(em.Cli, key, em.Id, true)
	if err != nil {
		if !strings.Contains(err.Error(), "exists") {
			e := cacheRec(vxlan.Attrs().Name, vxlan.SrcAddr.String())
			if e != nil {
				return logging.Errorf("etcd failed %v, cache failed %v (%v:%v)", err, e, key, em.Id)
			}
			logging.Verbosef("put %v:%v failed %v, cache the key", key, em.Id, err)
		}
	}
	return nil
}

func ParseVxlan(key, value []byte) (string, string) {
	k := string(key)
	return filepath.Base(filepath.Dir(k)), filepath.Base(k)
}

func cacheRec(vxlan, src string) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return logging.Errorf("create dir %v failed, %v", cacheDir, err)
	}
	lk, err := disk.NewFileLock(cacheDir)
	if err != nil {
		return logging.Errorf("create dir mutex in %v failed, %v", cacheDir, err)
	}
	lk.Lock()
	defer lk.Close()
	name := filepath.Join(cacheDir, vxlan)
	err = ioutil.WriteFile(name, []byte(src), 0666)
	if err != nil {
		return logging.Errorf("cache key %v, value: %v failed, %v", name, src, err)
	}
	return nil
}

func CacheToEtcd() error {
	_, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return logging.Errorf("there is something wrong with data dir %v, %v", cacheDir, err)
	}

	lk, err := disk.NewFileLock(cacheDir)
	if err != nil {
		return logging.Errorf("create dir mutex in %v failed, %v", cacheDir, err)
	}
	lk.Lock()
	defer lk.Close()

	files, err := ioutil.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return logging.Errorf("read dir %v failed, %v", cacheDir, err)
	}

	em, err := etcdv3.New()
	if err != nil {
		return err
	}
	defer em.Close() // make sure to close the client

	for _, file := range files {
		if !file.IsDir() {
			if file.Name() == "lock" {
				continue
			}
			cacheFile := filepath.Join(cacheDir, file.Name())
			v, err := ioutil.ReadFile(cacheFile)
			if err != nil {
				logging.Errorf("read file %v failed, %v", cacheFile, err)
				continue
			}
			value := strings.Trim(string(v), "\r\n\t ")

			key := filepath.Join(em.RootKeyDir, vxlanKeyDir, file.Name(), value)
			err = etcdv3.TransPutKey(em.Cli, key, em.Id, true)
			if err == nil {
				err = os.Remove(cacheFile)
				if err != nil {
					logging.Errorf("remove file %v failed, %v", cacheFile, err)
				}
			}
		}
	}
	return nil
}
