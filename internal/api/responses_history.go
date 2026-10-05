package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

// Preserve the entire hosted record as literal assistant context. In
// particular, nested $ref keys in search results are data, not attachments.
// This is independent of the active tool catalog and of the current model.
func responseHistoricalRecord(value any) aistudio.Part {
	raw, _ := json.Marshal(value)
	return aistudio.Part{Text: "Historical hosted-tool record (context only; do not re-execute):\n" + string(raw)}
}

func responseHostedHistoryParts(kind string, raw json.RawMessage) ([]aistudio.Part, error) {
	if kind != "image_generation_call" {
		return []aistudio.Part{responseHistoricalRecord(raw)}, nil
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	var result string
	if rawJSONConfigured(record["result"]) {
		if err := json.Unmarshal(record["result"], &result); err != nil {
			return nil, fmt.Errorf("result must be a base64 image string")
		}
	}
	if result == "" {
		return []aistudio.Part{responseHistoricalRecord(raw)}, nil
	}
	data, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		return nil, fmt.Errorf("result must be a base64 image string")
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, fmt.Errorf("result must contain image data")
	}
	// Keep metadata as text and pixels as an image rather than sending a huge
	// base64 blob in the language-model context.
	delete(record, "result")
	return []aistudio.Part{responseHistoricalRecord(record), {InlineData: &aistudio.Blob{MIME: mime, Data: data}}}, nil
}
