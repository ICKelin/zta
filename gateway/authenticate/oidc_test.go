package authenticate

import (
	convey "github.com/smartystreets/goconvey/convey"

	"testing"
)

func TestOIDCAuthenticate(t *testing.T) {
	convey.Convey("test oidc authenticate", t, func() {
		convey.Convey("test new oidc instance", func() {
			oidc, err := NewOIDCService([]byte("{}"))
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(oidc, convey.ShouldBeNil)
		})
	})
}
