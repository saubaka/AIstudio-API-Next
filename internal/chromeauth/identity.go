package chromeauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"google.golang.org/protobuf/encoding/protowire"
)

// Chromium's Gaia ListAccounts protocol. The first account is authuser=0,
// matching the account selected by the MakerSuite transport.
const googleAccountsURL = "https://accounts.google.com/ListAccounts?json=standard&laf=b64bin&source=ChromiumBrowser"

const googleAccountsJSONURL = "https://accounts.google.com/ListAccounts?json=standard&source=ChromiumBrowser"

func verifyGoogleIdentity(ctx context.Context, client *http.Client, state *aistudio.StorageState, userAgent string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	email, empty, err := requestGoogleIdentity(ctx, client, state, userAgent, googleAccountsURL)
	if err != nil && empty {
		// Confirm an empty binary response with the standard JSON representation
		// of the same session endpoint before rejecting a missing account.
		email, _, err = requestGoogleIdentity(ctx, client, state, userAgent, googleAccountsJSONURL)
	}
	return email, err
}

func requestGoogleIdentity(ctx context.Context, client *http.Client, state *aistudio.StorageState, userAgent, targetURL string) (string, bool, error) {
	cookie, err := state.CookieHeader(targetURL, time.Now())
	if err != nil {
		return "", false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(" "))
	if err != nil {
		return "", false, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://www.google.com")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Cookie", cookie)
	// Never follow a login redirect or forward credentials to its destination.
	identityClient := *client
	identityClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := identityClient.Do(request)
	if err != nil {
		return "", false, fmt.Errorf("查询 Google 会话邮箱失败，请确认账户代理可访问 accounts.google.com：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("查询 Google 会话邮箱返回 HTTP %d，请刷新 AI Studio 后重新复制 cURL", response.StatusCode)
	}
	const maxIdentityBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxIdentityBody+1))
	if err != nil || len(body) > maxIdentityBody {
		return "", false, fmt.Errorf("读取 Google 会话邮箱响应失败")
	}
	index, indexErr := strconv.Atoi(state.GoogleAuthUser())
	if indexErr != nil || index < 0 || index > 99 {
		return "", false, fmt.Errorf("Google 账户序号无效")
	}
	email, err := listAccountsEmailAt(body, index)
	if err != nil {
		return "", len(bytes.TrimSpace(body)) == 0, fmt.Errorf("无法确认所选 Google 账户邮箱（%v），请刷新目标账户的 AI Studio 页面后重新复制 cURL", err)
	}
	if err := mergeResponseCookies(state, response, targetURL); err != nil {
		return "", false, err
	}
	return email, false, nil
}

type googleAccountIdentity struct {
	email, id                  string
	valid, signedOut, verified bool
}

func (account googleAccountIdentity) verifiedEmail() (string, error) {
	email := normalizedEmail(account.email)
	if email == "" || account.id == "" || !account.valid || account.signedOut || !account.verified {
		return "", fmt.Errorf("Google 默认账户未登录或身份无效")
	}
	return email, nil
}

// Only structured server account metadata is accepted. Never scan page text
// for email addresses or choose a different account when the primary is invalid.
func listAccountsEmail(body []byte) (string, error) {
	return listAccountsEmailAt(body, 0)
}

func listAccountsEmailAt(body []byte, selected int) (string, error) {
	body = bytes.TrimSpace(body)
	if bytes.HasPrefix(body, []byte(")]}'")) {
		body = bytes.TrimSpace(body[4:])
	}
	if len(body) == 0 {
		return "", fmt.Errorf("Google 账户响应为空")
	}
	if body[0] == '[' {
		return legacyListAccountsEmailAt(body, selected)
	}
	if body[0] == '"' {
		var encoded string
		if json.Unmarshal(body, &encoded) != nil {
			return "", fmt.Errorf("Google 账户响应格式无效")
		}
		body = []byte(encoded)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(string(body))
	}
	if err != nil {
		return "", fmt.Errorf("Google 账户响应编码无效")
	}
	var primary *googleAccountIdentity
	index := 0
	for len(decoded) > 0 {
		number, kind, n := protowire.ConsumeTag(decoded)
		if n < 0 {
			return "", fmt.Errorf("Google 账户响应格式无效")
		}
		decoded = decoded[n:]
		if number == 1 {
			if kind != protowire.BytesType {
				return "", fmt.Errorf("Google 账户记录格式无效")
			}
			value, consumed := protowire.ConsumeBytes(decoded)
			if consumed < 0 {
				return "", fmt.Errorf("Google 账户记录不完整")
			}
			if index == selected {
				account, err := decodeGoogleAccount(value)
				if err != nil {
					return "", err
				}
				primary = &account
			}
			index++
			decoded = decoded[consumed:]
		} else {
			n = protowire.ConsumeFieldValue(number, kind, decoded)
			if n < 0 {
				return "", fmt.Errorf("Google 账户响应格式无效")
			}
			decoded = decoded[n:]
		}
	}
	if primary == nil {
		return "", fmt.Errorf("Google 会话没有账户")
	}
	return primary.verifiedEmail()
}

func decodeGoogleAccount(data []byte) (googleAccountIdentity, error) {
	account := googleAccountIdentity{valid: true, verified: true}
	for len(data) > 0 {
		number, kind, n := protowire.ConsumeTag(data)
		if n < 0 {
			return account, fmt.Errorf("Google 账户记录格式无效")
		}
		data = data[n:]
		switch number {
		case 3, 10:
			if kind != protowire.BytesType {
				return account, fmt.Errorf("Google 账户身份字段格式无效")
			}
			value, consumed := protowire.ConsumeString(data)
			if consumed < 0 {
				return account, fmt.Errorf("Google 账户身份字段不完整")
			}
			if number == 3 {
				account.email = value
			} else {
				account.id = value
			}
			data = data[consumed:]
		case 9, 14, 15:
			if kind != protowire.VarintType {
				return account, fmt.Errorf("Google 账户状态字段格式无效")
			}
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 || value > 1 {
				return account, fmt.Errorf("Google 账户状态字段无效")
			}
			switch number {
			case 9:
				account.valid = value == 1
			case 14:
				account.signedOut = value == 1
			case 15:
				account.verified = value == 1
			}
			data = data[consumed:]
		default:
			n = protowire.ConsumeFieldValue(number, kind, data)
			if n < 0 {
				return account, fmt.Errorf("Google 账户记录格式无效")
			}
			data = data[n:]
		}
	}
	return account, nil
}

func legacyListAccountsEmail(body []byte) (string, error) {
	return legacyListAccountsEmailAt(body, 0)
}

func legacyListAccountsEmailAt(body []byte, selected int) (string, error) {
	var root []json.RawMessage
	if json.Unmarshal(body, &root) != nil || len(root) < 2 {
		return "", fmt.Errorf("Google 账户响应格式无效")
	}
	var rows [][]json.RawMessage
	if json.Unmarshal(root[1], &rows) != nil || (len(rows) > 0 && len(rows[0]) < 11) {
		return "", fmt.Errorf("Google 账户记录格式无效")
	}
	if selected < 0 || selected >= len(rows) {
		return "", fmt.Errorf("Google 未返回已登录账户，Cookie 可能过期或导出不完整")
	}
	row := rows[selected]
	if len(row) < 11 {
		return "", fmt.Errorf("Google 账户记录格式无效")
	}
	account := googleAccountIdentity{valid: true, verified: true}
	if json.Unmarshal(row[3], &account.email) != nil || json.Unmarshal(row[10], &account.id) != nil {
		return "", fmt.Errorf("Google 账户身份字段格式无效")
	}
	for _, field := range []struct {
		index int
		flag  *bool
	}{{9, &account.valid}, {14, &account.signedOut}, {15, &account.verified}} {
		if field.index >= len(row) || bytes.Equal(row[field.index], []byte("null")) {
			continue
		}
		var value int
		if json.Unmarshal(row[field.index], &value) != nil || (value != 0 && value != 1) {
			return "", fmt.Errorf("Google 账户状态字段无效")
		}
		*field.flag = value == 1
	}
	return account.verifiedEmail()
}
