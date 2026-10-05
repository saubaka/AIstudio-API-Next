package api

import (
	"net/http"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

// channelMiddleware runs inside the standard authentication/origin boundary.
// Prefix stripping changes routing only; access logs retain the external path.
func channelMiddleware(channel aistudio.Channel, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-AIStudio-Channel", string(channel))
		if channel == aistudio.ChannelBuild && !buildEndpointSupported(r) {
			message := "App Build does not support this endpoint; use the Playground endpoint"
			switch protocolForRequest(r) {
			case "gemini":
				writeGeminiError(w, http.StatusNotImplemented, "UNIMPLEMENTED", message)
			case "anthropic":
				writeAnthropicError(w, http.StatusNotImplemented, "not_supported_error", message)
			default:
				writeOpenAIError(w, http.StatusNotImplemented, "channel_not_supported", message)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(aistudio.ContextWithChannel(r.Context(), channel)))
	})
}

func buildEndpointSupported(r *http.Request) bool {
	path := r.URL.Path
	if path == "/v1/files" || strings.HasPrefix(path, "/v1/files/") || strings.HasPrefix(path, "/v1/uploads/") {
		return true
	}
	if r.Method == http.MethodGet {
		return path == "/v1/models" || path == "/v1beta/models" || strings.HasPrefix(path, "/v1beta/models/")
	}
	if r.Method != http.MethodPost {
		return false
	}
	switch path {
	case "/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/images/generations", "/v1/audio/speech":
		return true
	}
	return strings.HasPrefix(path, "/v1beta/models/") &&
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent"))
}

func (s *server) handleChannelModels(w http.ResponseWriter, r *http.Request) {
	channel := aistudio.Channel(r.PathValue("channel"))
	if channel != aistudio.ChannelPlayground && channel != aistudio.ChannelBuild {
		writeOpenAIError(w, http.StatusNotFound, "channel_not_found", "Unknown upstream channel")
		return
	}
	models, err := s.config.Admin.Models(r.Context())
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": channel, "models": aistudio.ModelsForChannel(models, channel)})
}
