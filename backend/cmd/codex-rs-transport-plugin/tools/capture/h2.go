package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const http2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func settingName(id http2.SettingID) string {
	switch id {
	case http2.SettingHeaderTableSize:
		return "HEADER_TABLE_SIZE"
	case http2.SettingEnablePush:
		return "ENABLE_PUSH"
	case http2.SettingMaxConcurrentStreams:
		return "MAX_CONCURRENT_STREAMS"
	case http2.SettingInitialWindowSize:
		return "INITIAL_WINDOW_SIZE"
	case http2.SettingMaxFrameSize:
		return "MAX_FRAME_SIZE"
	case http2.SettingMaxHeaderListSize:
		return "MAX_HEADER_LIST_SIZE"
	default:
		return fmt.Sprintf("UNKNOWN_%d", id)
	}
}

func serveHTTP2Capture(rw io.ReadWriter, h2 *portrait.HTTP2Capture) (requests int, err error) {
	preface := make([]byte, len(http2Preface))
	if _, err := io.ReadFull(rw, preface); err != nil {
		return 0, fmt.Errorf("read http2 preface: %w", err)
	}
	if string(preface) != http2Preface {
		return 0, fmt.Errorf("invalid http2 preface")
	}
	framer := http2.NewFramer(rw, rw)
	framer.AllowIllegalWrites = true
	decoder := hpack.NewDecoder(4096, nil)
	gotSettings := false
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			if requests > 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
				return requests, nil
			}
			return requests, err
		}
		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if f.IsAck() {
				continue
			}
			if !gotSettings {
				for i := 0; i < f.NumSettings(); i++ {
					s := f.Setting(i)
					h2.Settings = append(h2.Settings, portrait.H2Setting{
						ID:    uint16(s.ID),
						Name:  settingName(s.ID),
						Value: s.Val,
					})
					h2.SettingsOrder = append(h2.SettingsOrder, uint16(s.ID))
					switch s.ID {
					case http2.SettingHeaderTableSize:
						// recorded in Settings
					case http2.SettingInitialWindowSize:
						h2.StreamInitialWindow = s.Val
					case http2.SettingMaxFrameSize:
						h2.MaxFrameSize = s.Val
					case http2.SettingMaxHeaderListSize:
						h2.MaxHeaderListSize = s.Val
					}
				}
				gotSettings = true
			}
			if err := framer.WriteSettings(http2.Setting{ID: http2.SettingEnablePush, Val: 0}); err != nil {
				return requests, err
			}
			if err := framer.WriteSettingsAck(); err != nil {
				return requests, err
			}
		case *http2.WindowUpdateFrame:
			if f.Header().StreamID == 0 && h2.ConnectionWindowUpdate == 0 {
				h2.ConnectionWindowUpdate = f.Increment
			}
		case *http2.PriorityFrame:
			h2.PriorityFrames = append(h2.PriorityFrames, "PRIORITY")
		case *http2.HeadersFrame:
			fields, decErr := decoder.DecodeFull(f.HeaderBlockFragment())
			if decErr != nil {
				return requests, fmt.Errorf("hpack: %w", decErr)
			}
			path := ""
			if len(h2.PseudoHeaderOrder) == 0 {
				for _, hf := range fields {
					if strings.HasPrefix(hf.Name, ":") {
						h2.PseudoHeaderOrder = append(h2.PseudoHeaderOrder, hf.Name)
					}
				}
			}
			for _, hf := range fields {
				if hf.Name == ":path" {
					path = hf.Value
				}
			}
			streamID := f.Header().StreamID
			if err := writeInspectorResponse(framer, streamID, path); err != nil {
				return requests, err
			}
			requests++
		case *http2.DataFrame:
			// body discarded; never written to capture artifacts
		case *http2.GoAwayFrame:
			return requests, nil
		}
	}
}

func writeInspectorResponse(framer *http2.Framer, streamID uint32, path string) error {
	var buf []byte
	enc := hpack.NewEncoder(&sliceBuffer{b: &buf})
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/json"})
	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: buf,
		EndHeaders:    true,
		EndStream:     false,
	}); err != nil {
		return err
	}
	return framer.WriteData(streamID, true, inspectorJSON(path))
}

func inspectorJSON(path string) []byte {
	if strings.Contains(path, "model") {
		return []byte(`{"models":[],"object":"list"}`)
	}
	return []byte(`{"ok":true,"inspector":"codex-rs-transport-plugin"}`)
}

type sliceBuffer struct{ b *[]byte }

func (s *sliceBuffer) Write(p []byte) (int, error) {
	*s.b = append(*s.b, p...)
	return len(p), nil
}
