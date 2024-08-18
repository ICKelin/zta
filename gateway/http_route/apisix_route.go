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

func (apisix *ApisixRouter) UpdateSSL(id, cert, key string, snis []string) error {
	reqForm := map[string]interface{}{
		"cert": cert,
		"key":  key,
		"snis": snis,
	}
	url := fmt.Sprintf("%s/apisix/admin/ssls/%s", apisix.conf.Api, id)
	err := apisix.doReq("PUT", url, reqForm)
	if err != nil {
		return fmt.Errorf("create ssl fail: %v", err)
	}
	return nil
}

func (apisix *ApisixRouter) UpdateRoute(param map[string]interface{}) error {
	url := fmt.Sprintf("%s/apisix/admin/routes", apisix.conf.Api)
	return apisix.doReq("PUT", url, param)
}

func (apisix *ApisixRouter) doReq(method, url string, reqForm interface{}) error {
	cli := &http.Client{
		Timeout: time.Second * 5,
	}

	body, err := json.Marshal(reqForm)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
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
