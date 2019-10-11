package backend

"github.com/containernetworking/cni/pkg/types"

type IPPool interface {
	ApplyIPRange(network string, subnet *types.IPNet, unit uint32) (net.IP, net.IP, error)
}