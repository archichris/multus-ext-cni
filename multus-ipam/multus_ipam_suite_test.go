package main

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMultusIpam(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MultusIpam Suite")
}
