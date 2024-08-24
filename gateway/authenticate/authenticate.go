package authenticate

type UserInfo struct {
	Username string
	Password string
	Email    string
}

type Authenticate interface {
	AddClient(clientID, clientSecret string)
	AddUser(clientID string, userInfo *UserInfo)
}

// The ID Token represents a JWT passed to the client as part of the token response.
//
// https://openid.net/specs/openid-connect-core-1_0.html#IDToken
type IDToken struct {
	Issuer     string `json:"iss"`
	UserID     string `json:"sub"`
	ClientID   string `json:"aud"`
	Expiration int64  `json:"exp"`
	IssuedAt   int64  `json:"iat"`
	Nonce      string `json:"nonce,omitempty"` // Non-manditory fields MUST be "omitempty"
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
}
