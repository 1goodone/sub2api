package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// validateCodexProbeResponse accepts only a completed Responses result. A clean
// EOF, an error event, an incomplete response, or a stream containing only
// [DONE] is not a valid ticket harvest.
func validateCodexProbeResponse(body []byte, expectedModel string) error {
	completed := false
	inspect := func(data []byte) error {
		data = bytes.TrimSpace(data)
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			return nil
		}
		var event struct {
			Model    string          `json:"model"`
			Type     string          `json:"type"`
			Status   string          `json:"status"`
			Error    json.RawMessage `json:"error"`
			Response *struct {
				Model  string `json:"model"`
				Status string `json:"status"`
			} `json:"response"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return errors.New("invalid probe event")
		}
		want := normalizeOpenAICodexTicketModel(expectedModel)
		for _, served := range []string{event.Model, func() string {
			if event.Response != nil {
				return event.Response.Model
			}
			return ""
		}()} {
			if served != "" && normalizeOpenAICodexTicketModel(served) != want {
				return errors.New("probe response model mismatch")
			}
		}
		if event.Type == "error" || event.Type == "response.failed" || event.Type == "response.incomplete" ||
			event.Status == "failed" || event.Status == "incomplete" ||
			(len(event.Error) > 0 && string(event.Error) != "null") {
			return errors.New("probe response failed")
		}
		if event.Response != nil && (event.Response.Status == "failed" || event.Response.Status == "incomplete") {
			return errors.New("probe response failed")
		}
		if event.Type == "response.completed" {
			if event.Response == nil || event.Response.Status != "completed" {
				return errors.New("invalid probe completion")
			}
			completed = true
		}
		if event.Type == "" && event.Status == "completed" {
			completed = true
		}
		return nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return errors.New("probe did not complete successfully")
	}
	if trimmed[0] == '{' {
		if err := inspect(trimmed); err != nil {
			return err
		}
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		var data []string
		flush := func() error {
			if len(data) == 0 {
				return nil
			}
			err := inspect([]byte(strings.Join(data, "\n")))
			data = nil
			return err
		}
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if line == "" {
				if err := flush(); err != nil {
					return err
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if err := scanner.Err(); err != nil {
			return errors.New("invalid probe stream")
		}
		if len(data) > 0 {
			return errors.New("unterminated probe event")
		}
	}
	if !completed {
		return errors.New("probe did not complete successfully")
	}
	return nil
}
