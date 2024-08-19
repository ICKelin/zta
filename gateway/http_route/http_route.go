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
	// UpdateSSL update route ssl certificate and key
	// id: uniq id
	// cert: ssl certificate content
	// key: ssl key content
	// snis: domains
	UpdateSSL(id, cert, key string, snis []string) error

	// UpdateRoute update http route rule
	// param: route configuration
	UpdateRoute(param map[string]interface{}) error
}

// InitRoute create global route instance base on routeType and configuration
// currently supports apisix only
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

// GetRoute get global route instance of routeType
func GetRoute(routeType string) HTTPRoute {
	return routes[routeType]
}
