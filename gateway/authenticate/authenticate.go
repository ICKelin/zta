package authenticate

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"gopkg.in/square/go-jose.v1"
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
		// TODO: parse config
		return runOIDCService()
	default:
		return errNotSupportedAuthType
	}
	return nil
}

func runOIDCService() error {
	// Load signing key.
	block, _ := pem.Decode(privateKeyBytes)
	if block == nil {
		return errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}

	// Configure jwtSigner and public keys.
	privateKey := &jose.JsonWebKey{
		Key:       key,
		Algorithm: "RS256",
		Use:       "sig",
		KeyID:     "1", // KeyID should use the key thumbprint.
	}

	jwtSigner, err := jose.NewSigner(jose.RS256, privateKey)
	if err != nil {
		return err
	}
	publicKeys := &jose.JsonWebKeySet{
		Keys: []jose.JsonWebKey{
			{
				Key:       &key.PublicKey,
				Algorithm: "RS256",
				Use:       "sig",
				KeyID:     "1",
			},
		},
	}

	oidc := NewOIDC(&OIDCConfig{
		Issuer:     "http://oidc.zta.beyondnetwork.net:14001",
		ListenAddr: ":14001",
	}, jwtSigner, publicKeys)

	oidc.AddClient("client_id", "client_secret", "http://app2.zta.beyondnetwork.net:9080/.apisix/redirect")
	oidc.AddUser("client_id", &UserInfo{
		Username: "username",
		Password: "password",
		Email:    "yingjiu.hulu@gmail.com",
	})
	go oidc.Serve()
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
