package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestMain initialise la police avant les tests — fontFace est un global requis par applyWatermark.
func TestMain(m *testing.M) {
	if err := loadFont(); err != nil {
		panic("loadFont: " + err.Error())
	}
	os.Exit(m.Run())
}

// ── adaptiveQuality ───────────────────────────────────────────────────────────

func TestAdaptiveQuality(t *testing.T) {
	cases := []struct {
		w, h int
		want int
	}{
		{100, 100, 80},   // 10 000 px < 250 000 → miniature
		{499, 499, 80},   // 249 001 px < 250 000 → miniature
		{500, 500, 85},   // 250 000 px — seuil bas → HD
		{800, 600, 85},   // 480 000 px → HD
		{1919, 1079, 85}, // juste sous Full HD → HD
		{1920, 1080, 90}, // exactement Full HD → default
		{2560, 1440, 90}, // au-delà → default
	}
	for _, c := range cases {
		got := adaptiveQuality(c.w, c.h)
		if got != c.want {
			t.Errorf("adaptiveQuality(%d,%d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

// ── wmCoords ──────────────────────────────────────────────────────────────────

func TestWmCoords(t *testing.T) {
	textW, imgW, imgH := 100, 1000, 800
	cases := []struct {
		position  string
		wantX, wantY int
	}{
		{"top-left",     wmMargin,                  wmLineHeight + wmMargin},
		{"top-right",    imgW - textW - wmMargin,   wmLineHeight + wmMargin},
		{"bottom-left",  wmMargin,                  imgH - wmMargin},
		{"bottom-right", imgW - textW - wmMargin,   imgH - wmMargin},
		{"unknown",      imgW - textW - wmMargin,   imgH - wmMargin}, // fallback → bottom-right
	}
	for _, c := range cases {
		x, y := wmCoords(textW, imgW, imgH, c.position)
		if x != c.wantX || y != c.wantY {
			t.Errorf("wmCoords(%q) = (%d,%d), want (%d,%d)", c.position, x, y, c.wantX, c.wantY)
		}
	}
}

// ── adaptiveColor ─────────────────────────────────────────────────────────────

func TestAdaptiveColorDarkBackground(t *testing.T) {
	// Image noire → fond sombre → texte blanc
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	// zero value = noir (tous les canaux à 0)
	col := adaptiveColor(img, 0, 60)
	if col.R != 255 || col.G != 255 || col.B != 255 {
		t.Errorf("fond sombre : attendu blanc (255,255,255), got (%d,%d,%d)", col.R, col.G, col.B)
	}
}

func TestAdaptiveColorLightBackground(t *testing.T) {
	// Image blanche → fond clair → texte gris foncé
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := range 100 {
		for x := range 300 {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	col := adaptiveColor(img, 0, 60)
	if col.R != 30 || col.G != 30 || col.B != 30 {
		t.Errorf("fond clair : attendu gris foncé (30,30,30), got (%d,%d,%d)", col.R, col.G, col.B)
	}
}

// ── sampleLuminance ───────────────────────────────────────────────────────────

func TestSampleLuminanceBlack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	// zero value = noir → luminance = 0
	lum := sampleLuminance(img, 0, 100)
	if lum != 0 {
		t.Errorf("image noire : luminance = %f, want 0", lum)
	}
}

func TestSampleLuminanceWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	for y := range 500 {
		for x := range 500 {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	lum := sampleLuminance(img, 0, 100)
	if lum < 254 {
		t.Errorf("image blanche : luminance = %f, want ~255", lum)
	}
}

func TestSampleLuminanceOutOfBounds(t *testing.T) {
	// Watermark positionné hors de l'image → zone vide → 0
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	lum := sampleLuminance(img, 200, 200)
	if lum != 0 {
		t.Errorf("hors image : luminance = %f, want 0", lum)
	}
}

func TestSampleLuminanceFewRows(t *testing.T) {
	// Chemin séquentiel (rows < numWorkers) — image trop petite pour paralléliser
	img := image.NewRGBA(image.Rect(0, 0, 300, 2))
	for x := range 300 {
		img.Set(x, 0, color.RGBA{200, 200, 200, 255})
		img.Set(x, 1, color.RGBA{200, 200, 200, 255})
	}
	lum := sampleLuminance(img, 0, 2)
	if lum < 190 || lum > 210 {
		t.Errorf("luminance grise : %f, want ~200", lum)
	}
}

// ── resize ────────────────────────────────────────────────────────────────────

func TestResizeNoOp(t *testing.T) {
	// Image déjà dans les limites → retourne le même pointeur
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	result := resize(img)
	if result != img {
		t.Error("resize doit retourner l'image originale quand déjà dans les limites")
	}
}

func TestResizeExactLimits(t *testing.T) {
	// Exactement maxWidth×maxHeight → pas de resize
	img := image.NewRGBA(image.Rect(0, 0, maxWidth, maxHeight))
	result := resize(img)
	if result != img {
		t.Error("resize ne doit pas modifier une image à exactement maxWidth×maxHeight")
	}
}

func TestResizeWide(t *testing.T) {
	// Plus large que maxWidth — la hauteur doit être réduite proportionnellement
	img := image.NewRGBA(image.Rect(0, 0, 3840, 1080))
	result := resize(img)
	w, h := result.Bounds().Dx(), result.Bounds().Dy()
	if w > maxWidth || h > maxHeight {
		t.Errorf("resize wide: %dx%d dépasse les limites %dx%d", w, h, maxWidth, maxHeight)
	}
	// Vérification du ratio (tolérance ±1% pour l'arrondi entier)
	origRatio := float64(3840) / float64(1080)
	newRatio := float64(w) / float64(h)
	diff := origRatio - newRatio
	if diff > 0.02 || diff < -0.02 {
		t.Errorf("ratio non conservé : orig=%.4f, new=%.4f (diff=%.4f)", origRatio, newRatio, diff)
	}
}

func TestResizeTall(t *testing.T) {
	// Plus haut que maxHeight
	img := image.NewRGBA(image.Rect(0, 0, 1000, 2160))
	result := resize(img)
	w, h := result.Bounds().Dx(), result.Bounds().Dy()
	if w > maxWidth || h > maxHeight {
		t.Errorf("resize tall: %dx%d dépasse les limites %dx%d", w, h, maxWidth, maxHeight)
	}
}

// ── applyWatermark ────────────────────────────────────────────────────────────

func TestApplyWatermarkDimensions(t *testing.T) {
	// Les dimensions de l'image ne doivent pas changer après le watermark
	img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	for y := range 800 {
		for x := range 1000 {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}
	result, err := applyWatermark(img, "Test Watermark", "bottom-right", "")
	if err != nil {
		t.Fatalf("applyWatermark: %v", err)
	}
	if result.Bounds() != img.Bounds() {
		t.Errorf("bounds modifiés : %v → %v", img.Bounds(), result.Bounds())
	}
}

func TestApplyWatermarkAllPositions(t *testing.T) {
	// Vérifier que toutes les positions ne paniquent pas
	img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	positions := []string{"top-left", "top-right", "bottom-left", "bottom-right"}
	for _, pos := range positions {
		_, err := applyWatermark(img, "NWS © 2026", pos, "")
		if err != nil {
			t.Errorf("applyWatermark(%q): %v", pos, err)
		}
	}
}

// ── encodeToBuffer ────────────────────────────────────────────────────────────

func TestEncodeToBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	buf, ct, q, err := encodeToBuffer(img)
	if err != nil {
		t.Fatalf("encodeToBuffer: %v", err)
	}
	defer bufPool.Put(buf)

	if ct != "image/jpeg" {
		t.Errorf("content-type = %q, want image/jpeg", ct)
	}
	if buf.Len() == 0 {
		t.Error("buffer vide")
	}
	// Vérifier que la qualité est dans la plage attendue
	if q < 75 || q > 95 {
		t.Errorf("qualité = %d, attendu entre 75 et 95", q)
	}
}

func TestEncodeToBufferQualityScaling(t *testing.T) {
	// Miniature → qualité 80, Full HD → qualité 90
	cases := []struct {
		w, h    int
		wantQ   int
	}{
		{100, 100, 80},
		{1920, 1080, 90},
	}
	for _, c := range cases {
		img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
		buf, _, q, err := encodeToBuffer(img)
		if err != nil {
			t.Fatalf("encodeToBuffer(%dx%d): %v", c.w, c.h, err)
		}
		bufPool.Put(buf)
		if q != c.wantQ {
			t.Errorf("encodeToBuffer(%dx%d) qualité = %d, want %d", c.w, c.h, q, c.wantQ)
		}
	}
}

// ── wmParams ──────────────────────────────────────────────────────────────────

func TestWmParamsDefaults(t *testing.T) {
	// Requête sans champs → valeurs par défaut
	r := httptest.NewRequest(http.MethodPost, "/optimize", nil)
	text, pos := wmParams(r)
	if text == "" {
		t.Error("wm_text vide : attendu la valeur par défaut")
	}
	if pos == "" {
		t.Error("wm_position vide : attendu la valeur par défaut")
	}
}

func TestWmParamsCustom(t *testing.T) {
	body := strings.NewReader("wm_text=Hello+World&wm_position=top-left")
	r := httptest.NewRequest(http.MethodPost, "/optimize", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	text, pos := wmParams(r)
	if text != "Hello World" {
		t.Errorf("wm_text = %q, want %q", text, "Hello World")
	}
	if pos != "top-left" {
		t.Errorf("wm_position = %q, want %q", pos, "top-left")
	}
}

// ── decodeImage ───────────────────────────────────────────────────────────────

// makeJPEGMultipart crée une requête multipart contenant une image JPEG encodée.
func makeJPEGMultipart(t *testing.T, w, h int) *http.Request {
	t.Helper()
	var imgBuf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := jpeg.Encode(&imgBuf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("image", "test.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(imgBuf.Bytes())
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/optimize", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestDecodeImageValid(t *testing.T) {
	r := makeJPEGMultipart(t, 200, 150)
	img, format, err := decodeImage(r)
	if err != nil {
		t.Fatalf("decodeImage: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("format = %q, want jpeg", format)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 150 {
		t.Errorf("dimensions = %dx%d, want 200x150", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestDecodeImageMissing(t *testing.T) {
	// Requête sans champ image → erreur
	r := httptest.NewRequest(http.MethodPost, "/optimize", nil)
	_, _, err := decodeImage(r)
	if err == nil {
		t.Error("attendu une erreur pour image manquante")
	}
}

func TestDecodeImageTooBig(t *testing.T) {
	// Image dépassant maxInputWidth × maxInputHeight → refus avant décodage complet
	r := makeJPEGMultipart(t, maxInputWidth+1, maxInputHeight+1)
	_, _, err := decodeImage(r)
	if err == nil {
		t.Error("attendu une erreur pour image trop grande")
	}
}

// ── formatBytes ───────────────────────────────────────────────────────────────

func TestFormatBytesOptimizer(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
	}
	for _, c := range cases {
		got := formatBytes(c.input)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.input, got, c.want)
		}
	}
}
