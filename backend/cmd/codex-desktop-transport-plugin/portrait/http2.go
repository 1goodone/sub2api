package portrait

import "golang.org/x/net/http2"

const (
	http2SettingMaxFrameSize      uint16 = 5
	http2SettingMaxHeaderListSize uint16 = 6
)

// ApplyMappedHTTP2Settings copies the subset of captured hyper SETTINGS that
// golang.org/x/net/http2.Transport can represent. Unknown IDs are ignored
// (see captures/http2_residuals.md). Empty captures are a no-op.
func ApplyMappedHTTP2Settings(tr *http2.Transport, p *Portrait) {
	if tr == nil || p == nil {
		return
	}
	if p.HTTP2.MaxFrameSize >= 16384 && p.HTTP2.MaxFrameSize <= 16777215 {
		tr.MaxReadFrameSize = uint32(p.HTTP2.MaxFrameSize)
	}
	if p.HTTP2.MaxHeaderListSize > 0 {
		tr.MaxHeaderListSize = p.HTTP2.MaxHeaderListSize
	}
	for _, setting := range p.HTTP2.Settings {
		switch setting.ID {
		case http2SettingMaxFrameSize:
			if setting.Value >= 16384 && setting.Value <= 16777215 {
				tr.MaxReadFrameSize = setting.Value
			}
		case http2SettingMaxHeaderListSize:
			if setting.Value > 0 {
				tr.MaxHeaderListSize = setting.Value
			}
		}
	}
}

func CaptureAllowsPool(p *Portrait) bool {
	return p != nil && p.Connection.Reuse == ReusePool
}
