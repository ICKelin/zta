package authenticate

import (
	"encoding/json"
	"errors"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jose-util/generator"
	"net/http"
	"os"
)

var (
	errNotSupportedAuthType    = errors.New("not supported auth type")
	errAuthTypeAlreadyRegister = errors.New("auth type already registered")
	authenticates              = make(map[string]Authenticate)
)

// UserInfo for authenticate user profile
type UserInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// Authenticate provides http authenticate
type Authenticate interface {
	// AddClient add a client(eg: apisix)
	AddClient(clientID, clientSecret, redirectUri string)
	// AddUser add a user into client
	AddUser(clientID string, userInfo *UserInfo)
}

// IDToken is the oidc id information reply for exchange code
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

// RunAuthenticateService base on config file
func RunAuthenticateService(confFile string) error {
	content, err := os.ReadFile(confFile)
	if err != nil {
		return err
	}

	var configs = make([]map[string]interface{}, 0)
	err = json.Unmarshal(content, &configs)
	if err != nil {
		return err
	}

	for _, config := range configs {
		configBytes, _ := json.Marshal(config)
		// just panic if type assert fail
		id := config["id"].(string)
		authType := config["type"].(string)
		err := runAuthenticateService(id, authType, configBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

// WatchConfigChanges watch for config file update interval
// support:
//   - add/del clients
//   - add/del client users
//
// TODO:
func WatchConfigChanges(confFile string) {
}

func runAuthenticateService(id, authType string, conf json.RawMessage) error {
	if _, ok := authenticates[authType]; ok {
		return errAuthTypeAlreadyRegister
	}

	switch authType {
	case "OIDC":
		oidc, err := NewOIDC(conf)
		if err != nil {
			return err
		}
		go oidc.Serve()
		authenticates[id] = oidc
	default:
		return errNotSupportedAuthType
	}
	return nil
}

func loadJws(privateKeyFile, publicKeyFile string) (jose.Signer, []byte, error) {
	content, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := generator.LoadPrivateKey(content)
	if err != nil {
		return nil, nil, err
	}

	content, err = os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, nil, err
	}

	publicKey, err := generator.LoadPublicKey(content)
	if err != nil {
		return nil, nil, err
	}

	jwtSigner, err := jose.NewSigner(jose.SigningKey{
		Algorithm: "RS256", // TODO
		Key:       privateKey,
	}, nil)
	if err != nil {
		return nil, nil, err
	}

	publicKeys := &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       publicKey,
				Algorithm: "RS256",
				Use:       "sig",
			},
		},
	}
	publicKeyBytes, err := json.Marshal(publicKeys)
	if err != nil {
		return nil, nil, err
	}

	return jwtSigner, publicKeyBytes, nil
}

type replyBody struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
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
