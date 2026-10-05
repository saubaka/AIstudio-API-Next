package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/api"
)

// loadHistory runs before serving requests. Migrate structured request records
// from the existing service log once; credentials and response bodies are absent.
func (registry *requestRegistry) loadHistory(path string) error {
	registry.historyPath = path
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.Open(path)
	legacy := errors.Is(err, os.ErrNotExist)
	if legacy {
		file, err = os.Open(filepath.Join(filepath.Dir(path), "logs", "service.log"))
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			var entry api.AdminLog
			if legacy {
				var old struct {
					Time    time.Time       `json:"time"`
					Level   string          `json:"level"`
					Message string          `json:"msg"`
					Event   string          `json:"event"`
					Source  string          `json:"source"`
					Request *api.RequestLog `json:"request"`
				}
				if json.Unmarshal(scanner.Bytes(), &old) != nil || old.Request == nil {
					continue
				}
				entry = api.AdminLog{Time: old.Time, Level: old.Level, Message: old.Message, Event: old.Event, Source: old.Source, Request: old.Request}
			} else if json.Unmarshal(scanner.Bytes(), &entry) != nil {
				continue
			}
			registry.logs = append(registry.logs, entry)
			if len(registry.logs) >= adminLogCompactAt {
				registry.logs = append([]api.AdminLog(nil), registry.logs[len(registry.logs)-adminLogRetain:]...)
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}
	if len(registry.logs) > adminLogRetain {
		registry.logs = registry.logs[len(registry.logs)-adminLogRetain:]
	}
	return registry.rewriteHistoryLocked(registry.logs)
}

func (registry *requestRegistry) appendHistoryLocked(entry api.AdminLog) error {
	if registry.historyPath == "" {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(registry.historyPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return errors.Join(err, file.Close())
}

func (registry *requestRegistry) rewriteHistoryLocked(entries []api.AdminLog) error {
	if registry.historyPath == "" {
		return nil
	}
	file, err := os.CreateTemp(filepath.Dir(registry.historyPath), ".request-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return errors.Join(err, file.Close())
		}
	}
	err = file.Sync()
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), registry.historyPath)
}
