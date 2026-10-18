package handler

import (
	"bytes"
	"database/sql"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"

	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxAvatarBytes = 2 << 20

func normalizeAvatar(data []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 4_194_304 {
		return nil, image.ErrFormat
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err = png.Encode(&out, img); err != nil {
		return nil, err
	}
	if out.Len() > maxAvatarBytes {
		return nil, image.ErrFormat
	}
	return out.Bytes(), nil
}

func (h *Handler) registerAvatar(api *gin.RouterGroup) {
	api.PUT("/auth/avatar", requireUser(), func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes+65536)
		file, err := c.FormFile("avatar")
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if err != nil || file.Size > maxAvatarBytes {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		reader, err := file.Open()
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, maxAvatarBytes+1))
		if err != nil || len(data) > maxAvatarBytes {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		data, err = normalizeAvatar(data)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid_avatar"})
			return
		}
		url, err := h.store.SetAvatar(c.Request.Context(), currentUser(c).ID, data)
		if err == nil {
			audit.Describe(c, audit.Detail{TargetType: "user", TargetID: currentUser(c).ID, Changes: map[string]any{"avatar_updated": true}})
		}
		respond(c, gin.H{"avatar_url": url}, err)
	})
	api.DELETE("/auth/avatar", requireUser(), func(c *gin.Context) {
		err := h.store.RemoveAvatar(c.Request.Context(), currentUser(c).ID)
		if err == nil {
			audit.Describe(c, audit.Detail{TargetType: "user", TargetID: currentUser(c).ID, Changes: map[string]any{"avatar_removed": true}})
		}
		respond(c, gin.H{"avatar_url": ""}, err)
	})
	api.GET("/users/:id/avatar", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.Status(404)
			return
		}
		data, version, err := h.store.Avatar(c.Request.Context(), id.String())
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			respond(c, nil, err)
			return
		}
		if errors.Is(err, sql.ErrNoRows) || (c.Query("v") != "" && c.Query("v") != version) {
			c.Status(404)
			return
		}
		etag := "\"" + version + "\""
		c.Header("ETag", etag)
		if c.Query("v") == "" {
			c.Header("Cache-Control", "public, max-age=0, must-revalidate")
		} else {
			c.Header("Cache-Control", "public, max-age=3600")
		}
		c.Header("X-Content-Type-Options", "nosniff")
		if c.GetHeader("If-None-Match") == etag {
			c.Status(304)
			return
		}
		c.Data(200, "image/png", data)
	})
}
