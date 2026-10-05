package browserlogin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const LoginURL = "https://accounts.google.com/AccountChooser?continue=https%3A%2F%2Faistudio.google.com%2Fprompts%2Fnew_chat"

type Browser struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Automatic  bool   `json:"automatic"`
	path       string
	executable string
}

// Browsers discovers installed applications without reading their personal profiles.
func Browsers() []Browser {
	choices := []Browser{}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		for _, candidate := range []struct {
			id, name, app, binary string
			automatic             bool
		}{
			{"chrome", "Google Chrome", "Google Chrome.app", "Google Chrome", true},
			{"safari", "Safari", "Safari.app", "Safari", false},
			{"edge", "Microsoft Edge", "Microsoft Edge.app", "Microsoft Edge", true},
			{"firefox", "Firefox", "Firefox.app", "firefox", false},
			{"brave", "Brave", "Brave Browser.app", "Brave Browser", true},
		} {
			for _, root := range []string{"/Applications", "/System/Applications", filepath.Join(home, "Applications")} {
				app := filepath.Join(root, candidate.app)
				binary := filepath.Join(app, "Contents", "MacOS", candidate.binary)
				if info, err := os.Stat(binary); err == nil && !info.IsDir() {
					choices = append(choices, Browser{ID: candidate.id, Name: candidate.name, Automatic: candidate.automatic, path: app, executable: binary})
					break
				}
			}
		}
	} else {
		for _, candidate := range []struct {
			id, name  string
			binaries  []string
			automatic bool
		}{
			{"chrome", "Google Chrome", []string{"google-chrome", "google-chrome-stable", "chrome.exe"}, true},
			{"edge", "Microsoft Edge", []string{"microsoft-edge", "msedge.exe"}, true},
			{"firefox", "Firefox", []string{"firefox", "firefox.exe"}, false},
			{"brave", "Brave", []string{"brave-browser", "brave.exe"}, true},
		} {
			paths := append([]string{}, candidate.binaries...)
			if runtime.GOOS == "windows" {
				for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
					if root == "" {
						continue
					}
					for _, relative := range map[string][]string{
						"chrome":  {"Google/Chrome/Application/chrome.exe"},
						"edge":    {"Microsoft/Edge/Application/msedge.exe"},
						"firefox": {"Mozilla Firefox/firefox.exe"},
						"brave":   {"BraveSoftware/Brave-Browser/Application/brave.exe"},
					}[candidate.id] {
						paths = append(paths, filepath.Join(root, filepath.FromSlash(relative)))
					}
				}
			}
			for _, value := range paths {
				binary, err := exec.LookPath(value)
				if err == nil {
					choices = append(choices, Browser{ID: candidate.id, Name: candidate.name, Automatic: candidate.automatic, path: binary, executable: binary})
					break
				}
			}
		}
	}
	return choices
}

func openBrowser(ctx context.Context, browser Browser) error {
	if runtime.GOOS == "darwin" {
		if err := exec.CommandContext(ctx, "/usr/bin/open", "-a", browser.path, LoginURL).Run(); err != nil {
			return fmt.Errorf("无法打开 %s", browser.Name)
		}
		return nil
	}
	cmd := exec.Command(browser.executable, LoginURL)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法打开 %s", browser.Name)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
