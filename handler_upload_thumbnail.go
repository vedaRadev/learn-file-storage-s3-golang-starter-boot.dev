package main

import (
	"fmt"
	"net/http"
    "io"
    "encoding/base64"

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

    mediaType := fileHeader.Header.Get("Content-Type")
    data, err := io.ReadAll(file)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }

    thumbData := base64.StdEncoding.EncodeToString(data);
    thumbUrl := fmt.Sprintf("data:%s,base64,%s", mediaType, thumbData)
    video.ThumbnailURL = &thumbUrl
    err = cfg.db.UpdateVideo(video)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }

	respondWithJSON(w, http.StatusOK, video)
}
