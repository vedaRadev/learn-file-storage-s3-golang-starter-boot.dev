package main

import (
	"fmt"
	"net/http"
    "bytes"
    "io"
    "strings"
    "mime"
    "os"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}


	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

    video, err := cfg.db.GetVideo(videoID)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }
    if video.UserID != userID {
        respondWithJSON(w, http.StatusUnauthorized, struct {}{})
    }

    const maxMemory int64 = 10 << 20 // 10 MB
    r.ParseMultipartForm(maxMemory)
    file, fileHeader, err := r.FormFile("thumbnail")
    if err != nil {
        respondWithJSON(w, http.StatusBadRequest, struct{}{})
        return
    }

    mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
    if mediaType != "image/jpeg" && mediaType != "image/png" {
        respondWithError(w, http.StatusBadRequest, "invalid thumbnail format", nil)
        return
    }
    data, err := io.ReadAll(file)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }

    extension := strings.Split(mediaType, "/")[1]
    assetPath := fmt.Sprintf("%s.%s", videoIDString, extension)
    assetDiskPath := fmt.Sprintf("%s/%s", cfg.assetsRoot, assetPath)
    assetFile, err := os.Create(assetDiskPath)
    fmt.Printf("creating file for thumb: %s\n", assetDiskPath)
    if err != nil {
        fmt.Printf("failed to create file for thumb: %s\n", err.Error())
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }
    _, err = io.Copy(assetFile, bytes.NewBuffer(data))
    if err != nil {
        fmt.Printf("failed to copy to file: %s\n", err.Error())
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }

    assetURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
    video.ThumbnailURL = &assetURL
    err = cfg.db.UpdateVideo(video)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }

	respondWithJSON(w, http.StatusOK, video)
}
