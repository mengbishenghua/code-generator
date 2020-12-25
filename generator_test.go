package main

import (
	"testing"
)

const DNS = "root:root@tcp(localhost:3306)/blog?charset=utf8&parseTime=true"

func TestGenerator(t *testing.T) {
	SetDNS(DNS)
	//SetPath("F:\\generator")
	//SetPackageName("model")
	SetDatabase("blog")
	ConnectionDatabase()
	Execute()
}
