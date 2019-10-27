package ipaddr_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestIpaddr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ipaddr Suite")
}
