module github.com/intel/multus-cni

go 1.12

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/alexflint/go-filemutex v0.0.0-20171022225611-72bdc8eae2ae
	github.com/archichris/netools/dev v0.0.0-20191123124102-ac7dd0b8116b
	github.com/archichris/netools/ipaddr v0.0.0-20191123124102-ac7dd0b8116b
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.8.2
	github.com/coreos/etcd v3.3.10+incompatible
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/fscrypt v0.2.5 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.2 // indirect
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/j-keck/arping v0.0.0-20160618110441-2cf9dc699c56
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.1
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.1.0 // indirect
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/spf13/cobra v0.0.5 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/vishvananda/netlink v1.0.1-0.20191217171528-ed8931371a80
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	google.golang.org/grpc v1.27.0
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
	k8s.io/api v0.20.0-alpha.2
	k8s.io/apimachinery v0.20.0-alpha.2
	k8s.io/client-go v0.20.0-alpha.2
	k8s.io/klog v0.0.0-20181108234604-8139d8cb77af // indirect
	k8s.io/kubernetes v1.13.0
)

replace github.com/docker/docker v1.13.1 => github.com/docker/engine v1.4.2-0.20180816081446-320063a2ad06

replace github.com/archichris/netools/dev => /opt/go/src/github.com/archichris/netools/dev
