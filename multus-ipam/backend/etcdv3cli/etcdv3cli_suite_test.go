package etcdv3cli

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEtcdv3cli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcdv3cli Suite")
}