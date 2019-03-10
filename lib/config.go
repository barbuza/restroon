package roonlib

import (
	"io/ioutil"

	"github.com/hashicorp/hcl"
)

type roonConfig struct {
	Logging struct {
		Level     string
		Formatter string
	}

	Roon struct {
		IP    string
		Port  int64
		Zones []string
		Token string
	}

	Rest struct {
		IP   string
		Port int64
	}
}

var Cfg roonConfig

func ParseConfig() {
	data, err := ioutil.ReadFile("config.hcl")
	if err != nil {
		panic(err)
	}
	err = hcl.Decode(&Cfg, string(data))
	if err != nil {
		panic(err)
	}
}
