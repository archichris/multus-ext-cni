package vxlan

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSimpleVxlan(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleVxlan Suite")
}
