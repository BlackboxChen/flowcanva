package main

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                string
	DBPath              string
	DevMode             bool
	Embedded            bool
	ConsoleDir          string
	CanvasDir           string
	UploadDir           string
	AdminDevURL         string
	CanvasDevURL        string
	ZhengAPIBaseURL     string
	ZhengAPIAPIKey      string
	ZhengAPIUseDataURLs bool
	MegaBYBaseURL       string
	MegaBYAPIKey        string
	GeekNowBaseURL      string
	GeekNowAPIKey       string
	GeekNowVideoAPIKey  string
	DeepSeekBaseURL     string
	DeepSeekAPIKey      string
	OSSAccessKeyID      string
	OSSAccessKeySecret  string
	OSSBucket           string
	OSSRegion           string
	OSSEndpoint         string
}

func loadConfig() Config {
	loadEnvFiles()
	embedded := os.Getenv("EMBEDDED") == "true"
	if !embedded {
		embedded = embedHasContent()
	}
	return Config{
		Port:                envOrDefault("PORT", ":16789"),
		DBPath:              envOrDefault("DB_PATH", "./data.db"),
		DevMode:             os.Getenv("DEV_MODE") == "true",
		Embedded:            embedded,
		ConsoleDir:          envOrDefault("CONSOLE_DIR", "../frontend-admin/dist"),
		CanvasDir:           envOrDefault("CANVAS_DIR", "../frontend-canvas/dist"),
		UploadDir:           envOrDefault("UPLOAD_DIR", "./uploads"),
		AdminDevURL:         envOrDefault("ADMIN_DEV_URL", "http://localhost:5174"),
		CanvasDevURL:        envOrDefault("CANVAS_DEV_URL", "http://localhost:5173"),
		ZhengAPIBaseURL:     strings.TrimRight(os.Getenv("ZHENGAPI_BASE_URL"), "/"),
		ZhengAPIAPIKey:      os.Getenv("ZHENGAPI_API_KEY"),
		ZhengAPIUseDataURLs: envBool("ZHENGAPI_USE_DATA_URLS"),
		MegaBYBaseURL:       strings.TrimRight(os.Getenv("MEGABY_BASE_URL"), "/"),
		MegaBYAPIKey:        os.Getenv("MEGABY_API_KEY"),
		GeekNowBaseURL:      strings.TrimRight(os.Getenv("GEEKNOW_BASE_URL"), "/"),
		GeekNowAPIKey:       os.Getenv("GEEKNOW_API_KEY"),
		GeekNowVideoAPIKey:  os.Getenv("GEEKNOW_VIDEO_API_KEY"),
		DeepSeekBaseURL:     strings.TrimRight(os.Getenv("DEEPSEEK_BASE_URL"), "/"),
		DeepSeekAPIKey:      os.Getenv("DEEPSEEK_API_KEY"),
		OSSAccessKeyID:      os.Getenv("OSS_ACCESS_KEY_ID"),
		OSSAccessKeySecret:  os.Getenv("OSS_ACCESS_KEY_SECRET"),
		OSSBucket:           os.Getenv("OSS_BUCKET"),
		OSSRegion:           os.Getenv("OSS_REGION"),
		OSSEndpoint:         strings.TrimRight(os.Getenv("OSS_ENDPOINT"), "/"),
	}
}

// loadEnvFiles makes configuration independent of the process working directory.
// This matters for release builds started from a shortcut or another directory.
func loadEnvFiles() {
	paths := []string{".env", "../.env"}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		paths = append(paths, filepath.Join(dir, ".env"), filepath.Join(dir, "..", ".env"))
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		loadEnvFile(path)
	}
}

func embedHasContent() bool {
	f, err := AdminEmbedFS.Open("admin-dist/index.html")
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key)))
	return err == nil && value
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func initLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
