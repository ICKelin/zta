package authenticate

import (
	"encoding/json"
	"errors"
	"net/http"
)

var (
	errNotSupportedAuthType = errors.New("not supported auth type")
)

type UserInfo struct {
	Username string
	Password string
	Email    string
}

type Authenticate interface {
	AddClient(clientID, clientSecret, redirectUri string)
	AddUser(clientID string, userInfo *UserInfo)
}

type IDToken struct {
	Issuer     string `json:"iss"`
	UserID     string `json:"sub"`
	ClientID   string `json:"aud"`
	Expiration int64  `json:"exp"`
	IssuedAt   int64  `json:"iat"`
	Nonce      string `json:"nonce,omitempty"`
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
}

type replyBody struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

func RunAuthenticateService(authType string, conf json.RawMessage) error {
	switch authType {
	case "OIDC":
		oidc, err := NewOIDC(conf)
		if err != nil {
			return err
		}
		go oidc.Serve()
	default:
		return errNotSupportedAuthType
	}
	return nil
}

// reply to web browser
func replyToUserAgent(w http.ResponseWriter, data interface{}, err error) {
	body := &replyBody{}
	if err != nil {
		body.Code = 99999
		body.Data = nil
		body.Message = err.Error()
	} else {
		body.Code = 0
		body.Data = data
		body.Message = "success"
	}

	b, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
