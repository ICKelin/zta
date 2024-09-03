package main

import (
	"encoding/json"
	"github.com/alecthomas/gometalinter/_linters/src/gopkg.in/yaml.v2"
	"os"
)

type Config struct {
	AutoReload    bool           `yaml:"auto_reload"`
	GatewayConfig *GatewayConfig `yaml:"gateway"`
	// route config
	// eg:
	//apisix: |
	//	{
	//		"api": "",
	//		"key": ""
	//	}
	HttpRoutes map[string]string `yaml:"http_routes"`
	// authenticate config
	HTTPAuthenticate string `yaml:"http_authenticate"`
	ListenerFile     string `yaml:"listener_file"`
	SSLFile          string `yaml:"ssl_file"`
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
	ID               string                 `json:"id"`
	ClientID         string                 `json:"client_id"`
	PublicProtocol   string                 `json:"public_protocol"`
	PublicIP         string                 `json:"public_ip"`
	PublicPort       uint16                 `json:"public_port"`
	InternalProtocol string                 `json:"internal_protocol"`
	InternalIP       string                 `json:"internal_ip"`
	InternalPort     uint16                 `json:"internal_port"`
	HTTPRouteType    string                 `json:"http_route_type"`
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

type SSLConfig struct {
	ID            string   `json:"id"`
	HTTPRouteType string   `json:"http_route_type"`
	Cert          string   `json:"-"`
	Key           string   `json:"-"`
	CertFile      string   `json:"cert_file"`
	KeyFile       string   `json:"key_file"`
	SNIs          []string `json:"snis"`
}

func ParseSSLConfig(confFile string) ([]*SSLConfig, error) {
	content, err := os.ReadFile(confFile)
	if err != nil {
		return nil, err
	}

	var cfgs = make([]*SSLConfig, 0)
	err = json.Unmarshal(content, &cfgs)
	if err != nil {
		return nil, err
	}

	for _, cfg := range cfgs {
		// load cert and key from file
		crt, err := os.ReadFile(cfg.CertFile)
		if err != nil {
			return nil, err
		}
		key, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, err
		}
		cfg.Cert = string(crt)
		cfg.Key = string(key)
	}
	return cfgs, nil
}
