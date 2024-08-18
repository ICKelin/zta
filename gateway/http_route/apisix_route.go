package http_route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var _ HTTPRoute = &ApisixRouter{}

type ApisixConfig struct {
	Api string `json:"api"`
	Key string `json:"key"`
}

type ApisixRouter struct {
	conf *ApisixConfig
}

func NewApisixRoute(conf json.RawMessage) (*ApisixRouter, error) {
	var apisixConf = ApisixConfig{}
	err := json.Unmarshal(conf, &apisixConf)
	if err != nil {
		return nil, err
	}

	return &ApisixRouter{conf: &apisixConf}, nil
}

func (apisix *ApisixRouter) UpdateRoute(param map[string]interface{}) error {
	cli := &http.Client{
		Timeout: time.Second * 5,
	}

	url := fmt.Sprintf("%s/apisix/admin/routes", apisix.conf.Api)
	body, err := json.Marshal(param)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", apisix.conf.Key)

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid http code %d msg %s",
			resp.StatusCode, string(content))
	}
	return nil
}
