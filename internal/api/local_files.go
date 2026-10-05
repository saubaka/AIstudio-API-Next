package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

const localFileLimit int64 = 32 << 20

type localUploadTooLarge struct{}

func (*localUploadTooLarge) Error() string     { return "local upload exceeds 32 MiB" }
func (*localUploadTooLarge) HTTPStatus() int   { return http.StatusRequestEntityTooLarge }
func (*localUploadTooLarge) ErrorCode() string { return "file_too_large" }

var localFileID = regexp.MustCompile(`^file-local-[a-f0-9]{32}$`)

type localFileService struct {
	root     string
	upstream aistudio.Service
}

func (f *localFileService) legacy() (aistudio.FileService, error) {
	service, ok := f.upstream.(aistudio.FileService)
	if !ok {
		return nil, fmt.Errorf("file service unavailable")
	}
	return service, nil
}
func (f *localFileService) UploadFile(ctx context.Context, request aistudio.UploadRequest) (aistudio.FileRef, error) {
	if err := os.MkdirAll(f.root, 0700); err != nil {
		return aistudio.FileRef{}, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return aistudio.FileRef{}, err
	}
	id := "file-local-" + hex.EncodeToString(random)
	path := filepath.Join(f.root, id+".data")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return aistudio.FileRef{}, err
	}
	committed := false
	defer func() {
		out.Close()
		if !committed {
			os.Remove(path)
			os.Remove(filepath.Join(f.root, id+".json"))
		}
	}()
	n, err := io.Copy(out, io.LimitReader(request.Reader, localFileLimit+1))
	if err != nil {
		return aistudio.FileRef{}, err
	}
	if n > localFileLimit {
		return aistudio.FileRef{}, &localUploadTooLarge{}
	}
	if n == 0 {
		return aistudio.FileRef{}, fmt.Errorf("%w: file must not be empty", aistudio.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return aistudio.FileRef{}, err
	}
	if request.ResolvePurpose != nil {
		request.Purpose, err = request.ResolvePurpose(ctx)
		if err != nil {
			return aistudio.FileRef{}, err
		}
	}
	metadata := aistudio.FileMetadata{ID: id, Name: filepath.Base(request.Name), MIME: request.MIME, Purpose: request.Purpose, Size: n, CreatedAt: time.Now()}
	raw, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(f.root, id+".json"), raw, 0600); err != nil {
		return aistudio.FileRef{}, err
	}
	committed = true
	return aistudio.FileRef{ID: id, Name: metadata.Name, MIME: metadata.MIME}, nil
}
func (f *localFileService) FileMetadata(ctx context.Context, id string) (aistudio.FileMetadata, error) {
	if !strings.HasPrefix(id, "file-local-") {
		service, err := f.legacy()
		if err != nil {
			return aistudio.FileMetadata{}, err
		}
		return service.FileMetadata(ctx, id)
	}
	if !localFileID.MatchString(id) {
		return aistudio.FileMetadata{}, aistudio.ErrResourceNotFound
	}
	raw, err := os.ReadFile(filepath.Join(f.root, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return aistudio.FileMetadata{}, aistudio.ErrResourceNotFound
	}
	if err != nil {
		return aistudio.FileMetadata{}, err
	}
	var metadata aistudio.FileMetadata
	err = json.Unmarshal(raw, &metadata)
	return metadata, err
}
func (f *localFileService) DownloadFile(ctx context.Context, id string) (aistudio.MediaStream, error) {
	if !strings.HasPrefix(id, "file-local-") {
		service, err := f.legacy()
		if err != nil {
			return aistudio.MediaStream{}, err
		}
		return service.DownloadFile(ctx, id)
	}
	metadata, err := f.FileMetadata(ctx, id)
	if err != nil {
		return aistudio.MediaStream{}, err
	}
	file, err := os.Open(filepath.Join(f.root, id+".data"))
	if err != nil {
		return aistudio.MediaStream{}, err
	}
	return aistudio.MediaStream{Body: file, MIME: metadata.MIME, Name: metadata.Name, Size: metadata.Size}, nil
}
func (f *localFileService) DeleteFile(ctx context.Context, id string) error {
	if !strings.HasPrefix(id, "file-local-") {
		service, err := f.legacy()
		if err != nil {
			return err
		}
		return service.DeleteFile(ctx, id)
	}
	if _, err := f.FileMetadata(ctx, id); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(f.root, id+".json")); err != nil {
		return err
	}
	return os.Remove(filepath.Join(f.root, id+".data"))
}
func (s *server) handleLocalFileList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.localFiles.root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeOpenAIError(w, 500, "file_error", err.Error())
		return
	}
	data := []openAIFileObject{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		metadata, err := s.localFiles.FileMetadata(r.Context(), id)
		if err == nil {
			data = append(data, openAIFileResponse(metadata))
		}
	}
	sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
	writeJSON(w, 200, map[string]any{"object": "list", "data": data, "has_more": false})
}

// Convert local uploads before scheduling, so neither channel inherits a Drive
// account lease or silently falls back. Content is sent only when referenced.
func (s *server) resolveLocalContents(ctx context.Context, contents []aistudio.Content) ([]aistudio.Content, error) {
	contents = cloneResponseContents(contents)
	for i := range contents {
		for j := range contents[i].Parts {
			part := &contents[i].Parts[j]
			if part.File == nil || !strings.HasPrefix(part.File.ID, "file-local-") {
				continue
			}
			media, err := s.localFiles.DownloadFile(ctx, part.File.ID)
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(media.Body, localFileLimit+1))
			media.Body.Close()
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > localFileLimit {
				return nil, fmt.Errorf("%w: local file exceeds 32 MiB", aistudio.ErrInvalidArgument)
			}
			part.File = nil
			if strings.HasPrefix(media.MIME, "text/") || media.MIME == "application/json" {
				if !utf8.Valid(data) {
					return nil, fmt.Errorf("%w: text file must be UTF-8", aistudio.ErrInvalidArgument)
				}
				part.Text = "Uploaded file: " + media.Name + "\n" + string(data)
			} else {
				part.InlineData = &aistudio.Blob{MIME: media.MIME, Data: data}
			}
		}
	}
	return contents, nil
}
func (s *server) generate(ctx context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	contents, err := s.resolveLocalContents(ctx, request.Contents)
	if err != nil {
		return nil, err
	}
	request.Contents = contents
	return s.service.Generate(ctx, request)
}

func (s *server) countTokens(ctx context.Context, request aistudio.TokenCountRequest) (aistudio.TokenCount, error) {
	contents, err := s.resolveLocalContents(ctx, request.Contents)
	if err != nil {
		return aistudio.TokenCount{}, err
	}
	request.Contents = contents
	return s.service.CountTokens(ctx, request)
}
