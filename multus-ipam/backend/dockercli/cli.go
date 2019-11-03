package dockercli

import (
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/multus-ipam/backend/disk"
	"golang.org/x/net/context"
)

func IPAMCheckLocalIPs(dir string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return logging.Errorf("create docker cli failed, %v", err)
	}
	leases := disk.LoadAllLeases("", dir)
	for f, id := range leases {
		if id == "gateway" {
			continue
		}
		containers, err := cli.ContainerList(context.Background(),
			types.ContainerListOptions{Filters: filters.NewArgs(filters.KeyValuePair{"id", id})})
		if err != nil {
			logging.Debugf("list container %v failed, %v", id, err)
			continue
		}
		if len(containers) == 0 {
			network := filepath.Base(filepath.Dir(f))
			s, err := disk.New(network, "")
			if err != nil {
				logging.Debugf("create disk manager failed, %v", err)
				continue
			}
			s.Lock()
			curID := disk.GetID(f)
			if curID == id {
				os.Remove(f)
			}
			s.Unlock()
		}
	}
	return nil
}
