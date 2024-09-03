package authenticate

import (
	"encoding/json"
	"github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestRunAuthenticateService(t *testing.T) {
	convey.Convey("test run authenticate service", t, func() {
		convey.Convey("test config parse", func() {
			var configBytes = `[{
					"type": "OIDCService",
					"testkey":"test_value"
			}]`
			var configs = make([]map[string]interface{}, 0)
			err := json.Unmarshal([]byte(configBytes), &configs)
			convey.So(err, convey.ShouldBeNil)
			convey.So(configs, convey.ShouldNotBeNil)
			convey.So(configs[0]["type"], convey.ShouldEqual, "OIDCService")
			convey.So(configs[0]["testkey"], convey.ShouldEqual, "test_value")
			//convey.So(configs[0].RawMessage, convey.ShouldEqual, []byte(configBytes))
		})
	})
}
