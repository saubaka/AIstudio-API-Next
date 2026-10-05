package browserlogin

import (
	"encoding/json"
	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"testing"
)

func TestCurlSelectedGoogleAccountSurvivesStorage(t *testing.T) {
	for _, url := range []string{"https://aistudio.google.com/u/2/prompts/new_chat?pli=1", "https://aistudio.google.com/prompts/new_chat?authuser=2"} {
		state, err := ParseSessionData("curl '" + url + "' -H 'Cookie: " + testCookie + "'")
		if err != nil || state.GoogleAuthUser() != "2" {
			t.Fatalf("selection lost: %v", err)
		}
		data, _ := json.Marshal(state)
		var loaded aistudio.StorageState
		if err := json.Unmarshal(data, &loaded); err != nil || loaded.GoogleAuthUser() != "2" {
			t.Fatal("persisted selection lost")
		}
		reimported, err := ParseSessionData(string(data))
		if err != nil || reimported.GoogleAuthUser() != "2" {
			t.Fatal("session JSON reimport lost account selection", err)
		}
	}
	for _, command := range []string{
		"curl 'https://aistudio.google.com/u/2/prompts/new_chat' -H 'X-Goog-AuthUser: 0' -H 'Cookie: " + testCookie + "'",
		"curl 'https://aistudio.google.com/u/invalid/prompts/new_chat' -H 'Cookie: " + testCookie + "'",
		"curl 'https://aistudio.google.com/prompts/new_chat?authuser=100' -H 'Cookie: " + testCookie + "'",
	} {
		if _, err := ParseSessionData(command); err == nil {
			t.Fatal("ambiguous/invalid account selection accepted")
		}
	}
}
