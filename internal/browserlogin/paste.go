package browserlogin

import (
	"net/url"
	"strings"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

// PasteCheck contains only a summary. Cookie values are never returned.
type PasteCheck struct {
	Format      string   `json:"format"`
	CookieCount int      `json:"cookie_count"`
	Missing     []string `json:"missing"`
	Ready       bool     `json:"ready"`
}

func InspectPaste(data string) (PasteCheck, error) {
	state, err := ParseSessionData(data)
	if err != nil {
		return PasteCheck{}, err
	}
	check := PasteCheck{Format: "cookie", CookieCount: len(state.Cookies), Missing: []string{}}
	trimmed := strings.TrimSpace(data)
	if strings.HasPrefix(trimmed, "curl ") || strings.HasPrefix(trimmed, "curl\n") || strings.HasPrefix(trimmed, "curl\t") {
		check.Format = "curl"
	} else if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		check.Format = "json"
	}
	for _, name := range []string{"SAPISID", "__Secure-1PAPISID", "__Secure-3PAPISID"} {
		if value, ok := state.CookieValue(name, "https://aistudio.google.com/", time.Now()); !ok || value == "" {
			check.Missing = append(check.Missing, name)
		}
	}
	check.Ready = len(check.Missing) == 0
	return check, nil
}

// cookieFromCurl parses browser-generated text only. It never executes a command,
// expands variables, reads cookie files, or sends the copied request.
func cookieFromCurl(data string) (string, error) {
	cookie, _, err := sessionFromCurl(data)
	return cookie, err
}

func sessionFromCurl(data string) (string, string, error) {
	args, err := splitBrowserCommand(data)
	if err != nil || len(args) < 2 || args[0] != "curl" {
		return "", "", invalid("cURL 格式无法识别，请重新右键复制单个请求为 cURL")
	}
	var target string
	var cookies []string
	var selected string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		var option, value string
		switch {
		case arg == "-H" || arg == "--header" || arg == "-b" || arg == "--cookie" || arg == "--url":
			option = arg
			i++
			if i >= len(args) {
				return "", "", invalid("cURL 内容不完整，请重新复制")
			}
			value = args[i]
		case strings.HasPrefix(arg, "--header="):
			option, value = "-H", strings.TrimPrefix(arg, "--header=")
		case strings.HasPrefix(arg, "--cookie="):
			option, value = "-b", strings.TrimPrefix(arg, "--cookie=")
		case strings.HasPrefix(arg, "--url="):
			option, value = "--url", strings.TrimPrefix(arg, "--url=")
		case strings.HasPrefix(arg, "-H"):
			option, value = "-H", arg[2:]
		case strings.HasPrefix(arg, "-b"):
			option, value = "-b", arg[2:]
		case strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://"):
			option, value = "--url", arg
		}
		switch option {
		case "--url":
			if target != "" {
				return "", "", invalid("请只复制一个 AI Studio 请求，不要复制多个请求")
			}
			target = value
		case "-H", "--header":
			name, body, ok := strings.Cut(value, ":")
			if ok && strings.EqualFold(strings.TrimSpace(name), "Cookie") {
				cookies = append(cookies, strings.TrimSpace(body))
			}
			if ok && strings.EqualFold(strings.TrimSpace(name), "X-Goog-AuthUser") {
				selected = strings.TrimSpace(body)
			}
		case "-b", "--cookie":
			if !strings.Contains(value, "=") {
				return "", "", invalid("这里只接受 Cookie 内容，不读取 cURL 的 Cookie 文件")
			}
			cookies = append(cookies, value)
		}
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "aistudio.google.com" || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return "", "", invalid("这不是 AI Studio 请求，请在网络列表中选择 https://aistudio.google.com 的请求")
	}
	selections := []string{selected, parsed.Query().Get("authuser")}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) >= 2 && segments[0] == "u" {
		selections = append(selections, segments[1])
	}
	selected = ""
	for _, value := range selections {
		if value == "" {
			continue
		}
		if selected != "" && selected != value {
			return "", "", invalid("cURL 中的 Google 账户序号不一致，请重新复制目标账户请求")
		}
		selected = value
	}
	if selected == "" {
		selected = "0"
	}
	var check aistudio.StorageState
	if err := check.SetGoogleAuthUser(selected); err != nil {
		return "", "", invalid(err.Error())
	}
	if len(cookies) == 0 {
		return "", "", invalid("复制内容没有 Cookie：请确认已登录 AI Studio，刷新后重新复制请求；若浏览器省略 Cookie，请改复制 Cookie 请求头的完整值")
	}
	return strings.Join(cookies, "; "), selected, nil
}

// splitBrowserCommand handles POSIX quoting and line continuations emitted by
// Safari/Chrome on macOS. Shell operators outside quotes are rejected.
func splitBrowserCommand(input string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote byte
	started := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote == '\'' {
			if ch == '\'' {
				quote = 0
			} else {
				word.WriteByte(ch)
			}
			continue
		}
		if ch == '\\' {
			if i+1 >= len(input) {
				return nil, invalid("cURL 引号或换行不完整")
			}
			next := input[i+1]
			if next == '\r' && i+2 < len(input) && input[i+2] == '\n' {
				i += 2
				continue
			}
			if next == '\n' {
				i++
				continue
			}
			if quote == '"' && !strings.ContainsRune("\\\"$`", rune(next)) {
				word.WriteByte(ch)
				continue
			}
			i++
			word.WriteByte(next)
			started = true
			continue
		}
		if quote == '"' {
			if ch == '"' {
				quote = 0
			} else {
				word.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			started = true
		case ' ', '\t', '\r', '\n':
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		case ';', '|', '&', '<', '>', '`', '$', '\x00':
			return nil, invalid("请复制浏览器生成的单个 cURL 请求")
		default:
			word.WriteByte(ch)
			started = true
		}
	}
	if quote != 0 {
		return nil, invalid("cURL 引号不完整")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}
