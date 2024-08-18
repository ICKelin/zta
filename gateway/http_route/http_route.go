package http_route

import (
	"encoding/json"
	"fmt"
)

var (
	TypeApisix               = "apisix"
	TypeNginx                = "nginx"
	TypeCaddy                = "caddy"
	ErrRouteTypeNotSupported = fmt.Errorf("route type not supported")
	routes                   = make(map[string]HTTPRoute)
)

type HTTPRoute interface {
	UpdateRoute(param map[string]interface{}) error
}

func InitRoute(routeType string, conf json.RawMessage) error {
	if _, ok := routes[routeType]; ok {
		return nil
	}

	switch routeType {
	case TypeApisix:
		apisix, err := NewApisixRoute(conf)
		if err != nil {
			return err
		}
		routes[routeType] = apisix
		return nil
	default:
		return ErrRouteTypeNotSupported
	}
}

func GetRoute(routeType string) HTTPRoute {
	return routes[routeType]
}
