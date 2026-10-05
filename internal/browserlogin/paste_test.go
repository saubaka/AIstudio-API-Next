package browserlogin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBrowserCurlPaste(t *testing.T) {
	for name, command := range map[string]string{
		"Safari header": "curl 'https://aistudio.google.com/prompts/new_chat' \\\n  -X 'GET' \\\n  -H 'Cookie: " + testCookie + "' \\\n  -H 'User-Agent: Safari' --compressed",
		"Chrome cookie": "curl 'https://aistudio.google.com/prompts/new_chat' -b '" + testCookie + "' -H 'accept: text/html'",
		"double quotes": "curl --url=https://aistudio.google.com/prompts/new_chat --header=\"cookie: " + testCookie + "\"",
		"empty cookie":  "curl https://aistudio.google.com/prompts/new_chat --cookie '" + testCookie + "; blank=; trailing=x;'",
	} {
		t.Run(name, func(t *testing.T) {
			check, err := InspectPaste(command)
			if err != nil || !check.Ready || check.Format != "curl" || check.CookieCount < 4 {
				t.Fatal("browser export not recognized", check, err)
			}
			encoded, _ := json.Marshal(check)
			if strings.Contains(string(encoded), "fixture-") {
				t.Fatal("inspection revealed a Cookie value")
			}
		})
	}
	for _, command := range []string{
		"curl https://aistudio.google.com/prompts/new_chat",
		"curl https://accounts.google.com -b '" + testCookie + "'",
		"curl https://aistudio.google.com.evil.test -b '" + testCookie + "'",
		"curl https://aistudio.google.com:8443 -b '" + testCookie + "'",
		"curl https://aistudio.google.com -b /tmp/cookies",
		"curl https://aistudio.google.com -b '" + testCookie + "'; touch /tmp/never-run",
		"curl https://aistudio.google.com -b '" + testCookie + "' | sh",
		"curl https://aistudio.google.com -b $(cat /tmp/cookies)",
		"curl https://aistudio.google.com -b 'Cookie: truncated",
		"curl https://aistudio.google.com https://other.test -b '" + testCookie + "'",
		"curl https://aistudio.google.com -H 'Cookie: SID=x\nHost: evil.test'",
	} {
		if _, err := InspectPaste(command); err == nil {
			t.Fatal("accepted unsuitable command")
		}
	}
}

func TestPasteInspectionReportsMissingWithoutAuthentication(t *testing.T) {
	check, err := InspectPaste("SID=placeholder; SAPISID=placeholder")
	if err != nil || check.Ready || len(check.Missing) != 2 {
		t.Fatal("incomplete session not reported", check, err)
	}
	check, err = InspectPaste(`[{"name":"SAPISID","value":"expired","domain":".google.com","expires":0}]`)
	if err != nil || check.Ready || len(check.Missing) != 3 {
		t.Fatal("expired cookie reported as usable", check, err)
	}
	state, err := ParseSessionData("curl https://aistudio.google.com -b 'SID=$(this-is-literal); SAPISID=a; __Secure-1PAPISID=b; __Secure-3PAPISID=c'")
	if err != nil || state.Cookies[0].Value != "$(this-is-literal)" {
		t.Fatal("quoted content must stay literal", err)
	}
}
