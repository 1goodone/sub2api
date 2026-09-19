package service

import (
	"errors"
	"strings"

	"github.com/tidwall/gjson"
)

var errCodexTurnStateMissing = errors.New("upstream did not return turn-state")

func codexTurnStateFromMetadata(payload []byte) string {
	if !codexTurnStateMetadataEvent(gjson.GetBytes(payload, "type").String()) {
		return ""
	}
	state := ""
	gjson.GetBytes(payload, "headers").ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), openAICodexTurnStateHeader) && value.Type == gjson.String {
			state = value.String()
			return false
		}
		return true
	})
	return state
}
