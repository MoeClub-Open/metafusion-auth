package handler

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/gin-gonic/gin"
)

func avatarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestAvatarNormalizationRejectsExecutableOrInvalidImages(t *testing.T) {
	for _, data := range [][]byte{[]byte(`<svg onload="alert(1)"/>`), []byte("not an image"), {}} {
		if _, err := normalizeAvatar(data); err == nil {
			t.Fatal("accepted non-image")
		}
	}
	out, err := normalizeAvatar(avatarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "png" {
		t.Fatalf("normalized image: %s %v", format, err)
	}
}

func TestAvatarUploadReadAndRemovalAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	st.Tokens = newTestIssuer(t)
	r := gin.New()
	New(st).Register(r)
	id, _, bearer := insertChainUser(t, ctx, st, "user")
	upload := func(token string, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("avatar", "avatar.png")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
		form.Close()
		req := httptest.NewRequest(http.MethodPut, "/api/auth/avatar", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := upload("", avatarPNG(t)); w.Code != 401 {
		t.Fatalf("anonymous upload: %d", w.Code)
	}
	if w := upload(bearer, []byte("<svg/>")); w.Code != 400 {
		t.Fatalf("invalid upload: %d", w.Code)
	}
	w := upload(bearer, avatarPNG(t))
	if w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	profile, err := st.PublicProfile(ctx, id, "")
	if err != nil || !strings.HasPrefix(profile.User.AvatarURL, "/api/users/"+id+"/avatar?v=") {
		t.Fatalf("public avatar: %+v %v", profile, err)
	}
	me := &store.User{ID: id}
	if err := st.FillProfile(ctx, me); err != nil || me.AvatarURL != profile.User.AvatarURL {
		t.Fatalf("session avatar: %+v %v", me, err)
	}
	get := httptest.NewRequest(http.MethodGet, profile.User.AvatarURL, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, get)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("ETag") == "" {
		t.Fatalf("image response: %d %v", w.Code, w.Header())
	}
	get.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, get)
	if w.Code != 304 {
		t.Fatalf("conditional GET: %d", w.Code)
	}
	remove := httptest.NewRequest(http.MethodDelete, "/api/auth/avatar", nil)
	remove.Header.Set("Authorization", "Bearer "+bearer)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, remove)
	if w.Code != 200 {
		t.Fatalf("remove: %d", w.Code)
	}
	profile, err = st.PublicProfile(ctx, id, "")
	if err != nil || profile.User.AvatarURL != "" {
		t.Fatalf("removed image still advertised: %+v %v", profile, err)
	}
	get = httptest.NewRequest(http.MethodGet, "/api/users/"+id+"/avatar", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, get)
	if w.Code != 404 {
		t.Fatalf("removed image: %d", w.Code)
	}
}
