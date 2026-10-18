// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/png" // a profile's board.png
	"io/fs"
	"log"
	"net/http"
	"path"

	"github.com/we-are-mono/verso/internal/sensors"
)

// boardPhotoPrefix is where the boards' photographs are served.
const boardPhotoPrefix = "/assets/boards/"

// boardPhoto is a profile's photograph of its board (its folder's board.png),
// as the Hardware page shows it beside the facts of the box. Its address
// names its content, so a browser keeps it for good and a new photograph is a
// new address, as the stylesheet's is; its size goes with it, so the page
// keeps its place while it loads.
type boardPhoto struct {
	href          string
	width, height int
	data          []byte
}

// loadBoardPhotos reads every profile folder's board.png, keyed by the folder.
// A photograph that is not a PNG is left out, and said in the log: a
// profile is data, and one bad file never stops the shell.
func loadBoardPhotos(fsys fs.FS) map[string]boardPhoto {
	photos := map[string]boardPhoto{}
	files, err := fs.Glob(fsys, "*/board.png")
	if err != nil {
		return photos
	}
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			continue
		}
		size, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			log.Printf("verso: hardware: %s is not a PNG the page can show: %v", file, err)
			continue
		}
		sum := sha256.Sum256(data)
		photos[path.Dir(file)] = boardPhoto{
			href:  boardPhotoPrefix + hex.EncodeToString(sum[:])[:12] + ".png",
			width: size.Width, height: size.Height,
			data: data,
		}
	}
	return photos
}

// photoFor is the photograph of a profile's board, nil for a board with no
// profile or a profile that ships none.
func (s *Server) photoFor(profile *sensors.Profile) *boardPhoto {
	if profile == nil {
		return nil
	}
	if photo, ok := s.boardPhotos[profile.Dir]; ok {
		return &photo
	}
	return nil
}

// handleBoardPhoto serves a board's photograph, kept by the browser for good:
// its address changes with its content. Another build's address is not here.
func (s *Server) handleBoardPhoto(w http.ResponseWriter, r *http.Request) {
	for _, photo := range s.boardPhotos {
		if photo.href == r.URL.Path {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = w.Write(photo.data)
			return
		}
	}
	http.NotFound(w, r)
}
