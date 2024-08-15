package http_router

type HTTPRouter interface {
	UpdateRoute(param map[string]interface{}) error
}
