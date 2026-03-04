package main

import (
	"bytes"
	"compress/gzip"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── bestFormat ────────────────────────────────────────────────────────────────

func TestBestFormat(t *testing.T) {
	cases := []struct {
		accept string
		want   string
	}{
		{"image/webp,image/png", "webp"},
		{"image/png,image/jpeg", "jpeg"},
		{"", "jpeg"},
		{"image/webp", "webp"},
		{"IMAGE/WEBP", "jpeg"}, // case-sensitive — les navigateurs envoient en minuscules
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept", c.accept)
		got := bestFormat(r)
		if got != c.want {
			t.Errorf("bestFormat(%q) = %q, want %q", c.accept, got, c.want)
		}
	}
}

// ── detectContentType ─────────────────────────────────────────────────────────

func TestDetectContentType(t *testing.T) {
	// Magic bytes WebP : RIFF????WEBP (12 octets minimum)
	webp := []byte("RIFF\x00\x00\x00\x00WEBP")
	if got := detectContentType(webp); got != "image/webp" {
		t.Errorf("detectContentType(webp) = %q, want image/webp", got)
	}

	// JPEG commence par 0xFF 0xD8
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01}
	if got := detectContentType(jpeg); got != "image/jpeg" {
		t.Errorf("detectContentType(jpeg) = %q, want image/jpeg", got)
	}

	// Données trop courtes → fallback JPEG
	if got := detectContentType([]byte{0x01, 0x02}); got != "image/jpeg" {
		t.Errorf("detectContentType(short) = %q, want image/jpeg", got)
	}

	// RIFF sans WEBP (ex: WAV) → fallback JPEG
	wav := []byte("RIFF\x00\x00\x00\x00WAVE")
	if got := detectContentType(wav); got != "image/jpeg" {
		t.Errorf("detectContentType(wav) = %q, want image/jpeg", got)
	}
}

// ── fmtMs ─────────────────────────────────────────────────────────────────────

func TestFmtMs(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0.000"},
		{1500 * time.Microsecond, "1.500"},
		{12345 * time.Microsecond, "12.345"},
		{1 * time.Millisecond, "1.000"},
	}
	for _, c := range cases {
		got := fmtMs(c.d)
		if got != c.want {
			t.Errorf("fmtMs(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// ── formatBytes ───────────────────────────────────────────────────────────────

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{2048, "2.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{2 * 1024 * 1024, "2.0 MB"},
	}
	for _, c := range cases {
		got := formatBytes(c.input)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── sendResponse ──────────────────────────────────────────────────────────────

func TestSendResponseNoGzip(t *testing.T) {
	// 0xFF 0xD8 → JPEG
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x01, 0x02, 0x03}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	sendResponse(w, r, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if !bytes.Equal(w.Body.Bytes(), data) {
		t.Error("body mismatch")
	}
	if ce := w.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want vide", ce)
	}
}

func TestSendResponseGzip(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x01, 0x02, 0x03}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip, deflate")

	sendResponse(w, r, data)

	if ce := w.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", ce)
	}
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close()
	got, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("io.ReadAll gzip: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("corps décompressé différent des données originales")
	}
}

func TestSendResponseWebP(t *testing.T) {
	// Magic bytes RIFF????WEBP → Content-Type: image/webp
	data := []byte("RIFF\x00\x00\x00\x00WEBP")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	sendResponse(w, r, data)

	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("Content-Type = %q, want image/webp", ct)
	}
}

// ── corsMiddleware ────────────────────────────────────────────────────────────

func TestCorsMiddlewarePreflight(t *testing.T) {
	// OPTIONS doit court-circuiter le handler et retourner 204
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("le handler ne doit pas être appelé pour OPTIONS")
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/upload", nil)

	corsMiddleware(next).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
}

func TestCorsMiddlewarePassthrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/upload", nil)

	corsMiddleware(next).ServeHTTP(w, r)

	if !called {
		t.Error("le handler suivant n'a pas été appelé")
	}
	if got := w.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Error("Access-Control-Expose-Headers manquant")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods manquant")
	}
}

// ── handleUpload ──────────────────────────────────────────────────────────────

func TestHandleUploadMissingImage(t *testing.T) {
	// Requête sans champ image → 400
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/upload", nil)

	handleUpload(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleUploadWithMockOptimizer(t *testing.T) {
	// Créer une image JPEG minimale via la stdlib
	var imgBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if err := jpeg.Encode(&imgBuf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	imgData := imgBuf.Bytes()

	// Mock optimizer : retourne la même image sans traitement
	mockOptimizer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imgData)
	}))
	defer mockOptimizer.Close()

	// Pointer l'API vers le mock
	t.Setenv("OPTIMIZER_URL", mockOptimizer.URL)

	// Construire la requête multipart
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("image", "test.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(imgData)
	mw.WriteField("wm_text", "Test")
	mw.WriteField("wm_position", "bottom-right")
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	handleUpload(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("corps de réponse vide")
	}
}

func TestHandleUploadOptimizerDown(t *testing.T) {
	// Optimizer inaccessible → 502
	t.Setenv("OPTIMIZER_URL", "http://127.0.0.1:1") // port fermé

	var imgBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	jpeg.Encode(&imgBuf, img, nil)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("image", "test.jpg")
	part.Write(imgBuf.Bytes())
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	handleUpload(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}
