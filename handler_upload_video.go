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
    "os/exec"
    "bytes"
    "encoding/json"
    "math"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func getVideoAspectRatio(filePath string) (string, error) {
    cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
    stdoutBuf := bytes.NewBuffer([]byte{})
    cmd.Stdout = stdoutBuf
    err := cmd.Run();
    if err != nil { return "", err }
    jsonOut := make(map[string]any)
    err = json.Unmarshal(stdoutBuf.Bytes(), &jsonOut)
    if err != nil { return "", err }

    streams := jsonOut["streams"].([]any)
    firstStream := streams[0].(map[string]any)
    width := firstStream["width"].(float64)
    height := firstStream["height"].(float64)

    ratio := width / height
    var result string
    if math.Abs(ratio - 1.7777) < 0.01 {
        result = "landscape" // 16:9
    } else if math.Abs(ratio - 0.5625) < 0.01 {
        result = "portrait" // 9:16
    } else {
        result = "other"
    }

    return result, nil
}

func processVideoForFastStart(filePath string) (string, error) {
    outputPath := filePath + ".processing"
    cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)
    err := cmd.Run()
    if err != nil { return "", err }
    return outputPath, nil
}

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
    fmt.Printf("created temp file for video: %s\n", tempFile.Name());
    defer os.Remove(tempFile.Name());
    defer tempFile.Close()

    maxBytesReader := http.MaxBytesReader(w, videoFile, 1 << 30)
    bytesCopied, err := io.Copy(tempFile, maxBytesReader)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to copy to file", nil)
        return
    }
    fmt.Printf("copied %d bytes\n", bytesCopied)

    fastStartFileName, err := processVideoForFastStart(tempFile.Name())
    if err != nil {
        fmt.Printf("%s\n", err.Error())
        respondWithError(w, http.StatusInternalServerError, "failed to process video for fast start", err)
        return
    }
    fmt.Printf("create fast start video: %s\n", fastStartFileName);
    fastStartFile, err := os.Open(fastStartFileName)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to open fast start video file", err)
        return
    }
    defer os.Remove(fastStartFile.Name())
    defer fastStartFile.Close()

    aspectRatio, err := getVideoAspectRatio(fastStartFile.Name())
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to get video aspect ratio", err)
        return
    }

    byteSlice := make([]byte, 32, 32)
    rand.Read(byteSlice)
    fileKey := fmt.Sprintf("%s/%x.%s", aspectRatio, byteSlice, strings.Split(mediaType, "/")[1])
    fmt.Printf("%s\n", fileKey)
    fmt.Printf("%s\n", mediaType)
    input := s3.PutObjectInput {
        Bucket: &cfg.s3Bucket,
        Key: &fileKey,
        Body: fastStartFile,
        ContentType: &mediaType,
    }
    _, err = cfg.s3Client.PutObject(context.Background(), &input)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to upload video", err)
        return
    }

    videoUrl := fmt.Sprintf(
        "%s,%s",
        cfg.s3Bucket,
        fileKey,
    );
    video.VideoURL = &videoUrl
    fmt.Printf("new video url: %s\n", *video.VideoURL)
    err = cfg.db.UpdateVideo(video)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "failed to update video in db", err)
        return
    }
    presignedVideo, err := cfg.dbVideotoSignedVideo(video)
    if err != nil {
        fmt.Printf("failed to generate presigned video url: %s\n", err.Error())
        respondWithError(w, http.StatusInternalServerError, "failed to generate presigned video url", err)
        return
    }
    fmt.Printf("presigned url: %s\n", *presignedVideo.VideoURL)

    respondWithJSON(w, http.StatusOK, presignedVideo)
}
