package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

type mediaFixture struct {
	channelFixture
	request aistudio.GenerateRequest
}

func (f *mediaFixture) Generate(ctx context.Context, r aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	f.request = r
	return f.channelFixture.Generate(ctx, r)
}
func TestLocalUploadsReachBothChannelsAndSurviveRestart(t *testing.T) {
	root := t.TempDir()
	fixture := &mediaFixture{}
	handler := NewHandler(fixture, Config{APIKey: "key", UploadDirectory: root})
	for _, channel := range []string{"playground", "build"} {
		for _, kind := range []struct {
			kind, mime string
			data       []byte
		}{
			{"images", "image/png", []byte{137, 80, 78, 71}}, {"audio", "audio/wav", []byte("RIFF-WAVE")}, {"videos", "video/mp4", []byte("video fixture")}, {"files", "text/plain", []byte("FILE_42")},
		} {
			body := new(bytes.Buffer)
			writer := multipart.NewWriter(body)
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="file"; filename="sample"`)
			h.Set("Content-Type", kind.mime)
			part, _ := writer.CreatePart(h)
			part.Write(kind.data)
			writer.WriteField("purpose", "user_data")
			writer.Close()
			r := httptest.NewRequest("POST", "/"+channel+"/v1/uploads/"+kind.kind, body)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			r.Header.Set("Authorization", "Bearer key")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("upload %s %d %s", kind.kind, w.Code, w.Body.String())
			}
			var file openAIFileObject
			if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
				t.Fatal(err)
			}
			// A fresh handler must still resolve the persisted identifier.
			handler = NewHandler(fixture, Config{APIKey: "key", UploadDirectory: root})
			request := `{"model":"shared","input":[{"role":"user","content":[{"type":"input_text","text":"describe"},{"type":"input_file","file_id":"` + file.ID + `"}]}]}`
			r = httptest.NewRequest("POST", "/"+channel+"/v1/responses", strings.NewReader(request))
			r.Header.Set("Authorization", "Bearer key")
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("generate %d %s", w.Code, w.Body.String())
			}
			if fixture.channel != aistudio.Channel(channel) {
				t.Fatal("channel escaped")
			}
			parts := fixture.request.Contents[0].Parts
			if parts[1].File != nil {
				t.Fatal("Drive reference reached generation")
			}
			if kind.kind == "files" {
				if !strings.Contains(parts[1].Text, "FILE_42") {
					t.Fatal("text file lost")
				}
			} else if parts[1].InlineData == nil || parts[1].InlineData.MIME != kind.mime || !bytes.Equal(parts[1].InlineData.Data, kind.data) {
				t.Fatal("media bytes or MIME lost")
			}
			r = httptest.NewRequest("GET", "/"+channel+"/v1/files/"+file.ID+"/content", nil)
			r.Header.Set("Authorization", "Bearer key")
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), kind.data) {
				t.Fatal("file download failed")
			}
			r = httptest.NewRequest("DELETE", "/"+channel+"/v1/files/"+file.ID, nil)
			r.Header.Set("Authorization", "Bearer key")
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatal("delete failed")
			}
		}
	}
}

func TestLocalFilesAuthAndTraversal(t *testing.T) {
	fixture := &mediaFixture{}
	handler := NewHandler(fixture, Config{APIKey: "key", UploadDirectory: t.TempDir()})
	for _, channel := range []string{"playground", "build"} {
		for _, path := range []string{"/v1/files", "/v1/uploads/images", "/v1/uploads/audio", "/v1/uploads/videos", "/v1/uploads/files"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("POST", "/"+channel+path, nil))
			if w.Code != 401 {
				t.Fatalf("unauthenticated upload: %d", w.Code)
			}
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/"+channel+"/v1/files/file-local-invalid/content", nil)
		r.Header.Set("Authorization", "Bearer key")
		handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("invalid local ID %d", w.Code)
		}
	}
}
