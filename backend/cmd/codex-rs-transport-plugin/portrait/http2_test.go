package portrait

import (
	"testing"

	"golang.org/x/net/http2"
)

func TestApplyMappedHTTP2SettingsNoopOnEmpty(t *testing.T) {
	tr := &http2.Transport{}
	ApplyMappedHTTP2Settings(tr, &Portrait{})
	if tr.MaxReadFrameSize != 0 || tr.MaxHeaderListSize != 0 {
		t.Fatalf("%+v", tr)
	}
}

func TestApplyMappedHTTP2SettingsCopiesFixture(t *testing.T) {
	tr := &http2.Transport{}
	p := &Portrait{HTTP2: HTTP2Capture{
		MaxFrameSize:      32768,
		MaxHeaderListSize: 65536,
		Settings: []H2Setting{
			{ID: 5, Value: 32768},
			{ID: 6, Value: 65536},
			{ID: 2, Value: 0},
		},
	}}
	ApplyMappedHTTP2Settings(tr, p)
	if tr.MaxReadFrameSize != 32768 || tr.MaxHeaderListSize != 65536 {
		t.Fatalf("mapped %+v", tr)
	}
}

func TestCaptureAllowsPool(t *testing.T) {
	if CaptureAllowsPool(&Portrait{Connection: ConnCapture{Reuse: ReusePerRequest}}) {
		t.Fatal("per_request must not allow pool")
	}
}
