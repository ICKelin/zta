package main

import (
	"encoding/json"
	"github.com/alecthomas/gometalinter/_linters/src/gopkg.in/yaml.v2"
	"os"
)

type Config struct {
	GatewayConfig *GatewayConfig `yaml:"gateway"`
	ListenerFile  string         `yaml:"listener_file"`
}

type GatewayConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

func ParseConfig(confFile string) (*Config, error) {
	content, err := os.ReadFile(confFile)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(content, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

type ListenerConfig struct {
	ClientID         string                 `json:"client_id"`
	PublicProtocol   string                 `json:"public_protocol"`
	PublicIP         string                 `json:"public_ip"`
	PublicPort       uint16                 `json:"public_port"`
	InternalProtocol string                 `json:"internal_protocol"`
	InternalIP       string                 `json:"internal_ip"`
	InternalPort     uint16                 `json:"internal_port"`
	HTTPParam        map[string]interface{} `json:"http_param"`
}

func ParseListenerConfig(confFile string) ([]*ListenerConfig, error) {
	content, err := os.ReadFile(confFile)
	if err != nil {
		return nil, err
	}

	var cfg = make([]*ListenerConfig, 0)
	err = json.Unmarshal(content, &cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
