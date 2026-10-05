package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

type StartInput struct {
	BrowserID    string `json:"browser_id"`
	Mode         string `json:"mode"`
	AccountID    string `json:"account_id"`
	Proxy        string `json:"proxy"`
	BrowserProxy string `json:"-"`
	Locale       string `json:"locale"`
	Timezone     string `json:"timezone"`
}

type CompleteInput struct {
	Data string `json:"data"`
}

type Session struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	BrowserID   string    `json:"browser_id"`
	BrowserName string    `json:"browser_name"`
	Capture     string    `json:"capture"`
	ExpiresAt   time.Time `json:"expires_at"`
	Opened      bool      `json:"opened"`
}

type Result struct {
	State aistudio.StorageState
	Email string
	Input StartInput
}

type pending struct {
	mu sync.Mutex
	Session
	input     StartInput
	directory string
	cmd       *exec.Cmd
	done      chan struct{}
	timer     *time.Timer
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*pending
	root     string
	closed   bool
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string     { return e.Message }
func (e *Error) HTTPStatus() int   { return e.Status }
func (e *Error) ErrorCode() string { return "google_login_error" }
func invalid(message string) error { return &Error{400, message} }

func New(root string) *Manager {
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return &Manager{sessions: make(map[string]*pending), root: root}
}

func (m *Manager) Start(ctx context.Context, input StartInput) (Session, error) {
	if input.Mode != "open" && input.Mode != "link" {
		return Session{}, invalid("请选择打开浏览器或获取链接")
	}
	browser := Browser{ID: "link", Name: "登录链接"}
	if input.Mode == "open" {
		found := false
		for _, candidate := range Browsers() {
			if candidate.ID == input.BrowserID {
				browser = candidate
				found = true
				break
			}
		}
		if !found {
			return Session{}, invalid("所选浏览器未安装，请选择本机浏览器或获取登录链接")
		}
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return Session{}, err
	}
	s := &pending{Session: Session{ID: hex.EncodeToString(random), URL: LoginURL, BrowserID: browser.ID, BrowserName: browser.Name, Capture: "manual", ExpiresAt: time.Now().Add(15 * time.Minute)}, input: input}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Session{}, &Error{503, "本机登录服务已停止"}
	}
	if len(m.sessions) >= 4 {
		m.mu.Unlock()
		return Session{}, &Error{429, "已有多个登录窗口，请先完成或取消现有登录"}
	}
	m.sessions[s.ID] = s
	m.mu.Unlock()
	s.mu.Lock()
	ok := false
	defer func() {
		s.mu.Unlock()
		if !ok {
			m.Cancel(s.ID)
		}
	}()
	if input.Mode == "open" {
		if browser.Automatic {
			if err := os.MkdirAll(m.root, 0700); err != nil {
				return Session{}, fmt.Errorf("创建登录目录失败")
			}
			directory, err := os.MkdirTemp(m.root, "session-")
			if err != nil {
				return Session{}, fmt.Errorf("创建登录目录失败")
			}
			s.directory = directory
			args := []string{"--user-data-dir=" + directory, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1", "--no-first-run", "--no-default-browser-check", "--new-window"}
			if input.BrowserProxy != "" {
				args = append(args, "--proxy-server="+input.BrowserProxy)
			}
			args = append(args, LoginURL)
			s.cmd = exec.Command(browser.executable, args...)
			if err := s.cmd.Start(); err != nil {
				return Session{}, fmt.Errorf("无法启动 %s", browser.Name)
			}
			s.done = make(chan struct{})
			go func() { _ = s.cmd.Wait(); close(s.done) }()
			waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if _, _, err := endpoint(waitCtx, s.directory, s.done); err != nil {
				return Session{}, err
			}
			s.Capture = "automatic"
		} else if err := openBrowser(ctx, browser); err != nil {
			return Session{}, err
		}
		s.Opened = true
	}
	s.timer = time.AfterFunc(time.Until(s.ExpiresAt), func() { m.Cancel(s.ID) })
	ok = true
	return s.Session, nil
}

// Capture reads only the dedicated login window, or user-provided session data.
func (m *Manager) Capture(ctx context.Context, id string, input CompleteInput) (Result, error) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || !time.Now().Before(s.ExpiresAt) {
		m.Cancel(id)
		return Result{}, &Error{410, "登录已过期或取消，请重新打开登录"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var state aistudio.StorageState
	var email string
	var err error
	if strings.TrimSpace(input.Data) != "" {
		state, err = ParseSessionData(input.Data)
	} else if s.Capture == "automatic" {
		state, email, err = captureChrome(ctx, s.directory, s.done)
	} else {
		return Result{}, invalid("请导入已登录浏览器的 Cookie 或会话 JSON")
	}
	if err != nil {
		return Result{}, err
	}
	if _, err := aistudio.NewSigner().Sign(state); err != nil {
		return Result{}, invalid("会话不完整或已过期，请在 AI Studio 登录后重新导入完整 Cookie")
	}
	return Result{State: state, Email: email, Input: s.input}, nil
}

func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, ws, err := endpoint(ctx, s.directory, s.done)
		if err == nil {
			_ = cdp(ctx, ws, "Browser.close", nil, nil)
		}
		cancel()
		select {
		case <-s.done:
		case <-time.After(time.Second):
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	}
	if s.directory != "" {
		_ = os.RemoveAll(s.directory)
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Cancel(id)
	}
}
