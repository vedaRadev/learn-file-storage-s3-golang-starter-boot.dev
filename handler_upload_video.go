package main

import (
	"net/http"
    "mime"
    "fmt"
    "os"
    "io"
    "context"
    "crypto/rand"
    "strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
    videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)

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

    video, err := cfg.db.GetVideo(videoID)
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
        return
    }
    if video.UserID != userID {
        respondWithJSON(w, http.StatusUnauthorized, struct {}{})
    }

    videoFile, videoHeader, err := r.FormFile("video")
    if err != nil {
        respondWithJSON(w, http.StatusInternalServerError, struct{}{})
    }
    defer videoFile.Close()

    mediaType, _, err := mime.ParseMediaType(videoHeader.Header.Get("Content-Type"))
    if mediaType != "video/mp4" {
        respondWithError(w, http.StatusBadRequest, "video must be an mp4", nil)
        return
    }

    tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to create temp file", nil)
        return
    }
    defer os.Remove("tubely-upload.mp4");
    defer tempFile.Close()

    maxBytesReader := http.MaxBytesReader(w, videoFile, 1 << 30)
    bytesCopied, err := io.Copy(tempFile, maxBytesReader)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to copy to file", nil)
        return
    }
    fmt.Printf("copied %d bytes\n", bytesCopied)

    _, err = tempFile.Seek(0, io.SeekStart)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed seek", nil)
        return
    }

    byteSlice := make([]byte, 32, 32)
    rand.Read(byteSlice)
    fileKey := fmt.Sprintf("%x.%s", byteSlice, strings.Split(mediaType, "/")[1])
    fmt.Printf("%s\n", fileKey)
    fmt.Printf("%s\n", mediaType)
    input := s3.PutObjectInput {
        Bucket: &cfg.s3Bucket,
        Key: &fileKey,
        Body: tempFile,
        ContentType: &mediaType,
    }
    _, err = cfg.s3Client.PutObject(context.Background(), &input)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to upload video", err)
        return
    }

    videoUrl := fmt.Sprintf(
        "https://%s.s3.%s.amazonaws.com/%s",
        cfg.s3Bucket,
        cfg.s3Region,
        fileKey,
    )
    video.VideoURL = &videoUrl
    fmt.Printf("new video url: %s\n", videoUrl)
    err = cfg.db.UpdateVideo(video)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to update video in db", err)
        return
    }

    respondWithJSON(w, http.StatusOK, video)
}
