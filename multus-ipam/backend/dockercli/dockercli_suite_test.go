package dockercli

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDockercli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dockercli Suite")
}
