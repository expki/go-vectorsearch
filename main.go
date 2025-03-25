package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/static"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/expki/go-vectorsearch/server"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

func main() {
	appCtx, stopApp := context.WithCancel(context.Background())

	// Load config
	var configPath string = "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	log.Default().Printf("Config path: %s\n", configPath)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Default().Printf("Creating sample config: %s\n", configPath)
		err = config.CreateSample(configPath)
		if err != nil {
			log.Fatalf("CreateSample: %v", err)
		}
	}
	log.Default().Println("Reading config...")
	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("ReadFile %q: %v", configPath, err)
	}
	log.Default().Println("Parsing config...")
	cfg, err := config.ParseConfig(configRaw)
	if err != nil {
		log.Fatalf("ParseConfig: %v", err)
	}
	log.Default().Println("Loading TLS...")
	err = cfg.TLS.Configurate()
	if err != nil {
		log.Fatalf("Configurate: %v", err)
	}

	// Logger
	log.Default().Println("Setting log level:", cfg.LogLevel.String())
	logConf := zap.NewDevelopmentConfig()
	logConf.Level = cfg.LogLevel.Zap()
	l, err := logConf.Build()
	if err != nil {
		log.Fatalf("zap.NewDevelopment: %v", err)
	}
	logger.Initialize(l)
	defer l.Sync()

	// AI
	logger.Sugar().Info("Loading AI...")
	aiClient, err := ai.NewOllama(cfg.Ollama)
	if err != nil {
		logger.Sugar().Fatalf("ai.New: %v", err)
	}

	// Database
	logger.Sugar().Info("Loading database...")
	db, err := database.New(appCtx, cfg.Database, true)
	if err != nil {
		logger.Sugar().Fatalf("database.New: %v", err)
	}

	// Server
	logger.Sugar().Info("Loading Server...")
	srv := server.New(cfg.Ollama, db, aiClient)

	// Create mux
	mux := http.NewServeMux()

	// HTTP
	server := http.Server{
		Handler: mux,
		Addr:    cfg.Server.HttpAddress,
	}

	// HTTP2
	server2 := http.Server{
		Handler: mux,
		Addr:    cfg.Server.HttpsAddress,
		TLSConfig: &tls.Config{
			GetCertificate: cfg.TLS.GetCertificate,
			ClientAuth:     tls.NoClientCert,
			NextProtos:     []string{"h2", "http/1.1"}, // Enable HTTP/2
		},
	}
	err = http2.ConfigureServer(&server2, &http2.Server{})
	if err != nil {
		logger.Sugar().Fatalf("http2.Server: %v", err)
	}

	// Headers middleware
	middlewareHeaders := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// WASM headers
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
			h.ServeHTTP(w, r)
		})
	}

	// Decompression middleware
	middlewareDecompression := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Content-Encoding"), "zstd") {
				h.ServeHTTP(w, r)
				return
			}
			reader, err := zstd.NewReader(r.Body, zstd.WithDecoderLowmem(true))
			if err != nil {
				logger.Sugar().Errorf("Failed to create zstd reader: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			defer reader.Close()
			r.Body = &zstdRequestReader{ReadCloser: r.Body, Reader: reader}
			h.ServeHTTP(w, r)
		})
	}

	// Compression middleware
	middlewareCompression := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "zstd") {
				h.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Encoding", "zstd")
			encoder, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
			if err != nil {
				logger.Sugar().Errorf("Failed to create zstd encoder: %v", err)
				h.ServeHTTP(w, r)
				return
			}
			defer encoder.Close()
			zstrw := &zstdResponseWriter{ResponseWriter: w, Writer: encoder}
			h.ServeHTTP(zstrw, r)
		})
	}

	// Routes: API
	mux.Handle("/api/upload", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(srv.UploadHttp)))))
	mux.Handle("/api/search", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(srv.SearchHttp)))))
	mux.Handle("/api/chat", middlewareHeaders(middlewareDecompression(http.HandlerFunc(srv.ChatHttp))))

	mux.Handle("/api/categories", middlewareHeaders(middlewareDecompression(http.HandlerFunc(srv.FetchCategoryNamesHttp))))
	mux.Handle("/api/delete/owner", middlewareHeaders(middlewareDecompression(http.HandlerFunc(srv.DeleteOwnerHttp))))
	mux.Handle("/api/delete/category", middlewareHeaders(middlewareDecompression(http.HandlerFunc(srv.DeleteCategoryHttp))))
	mux.Handle("/api/delete/document", middlewareHeaders(middlewareDecompression(http.HandlerFunc(srv.DeleteDocumentHttp))))

	// Routes: Static
	mux.Handle("/", middlewareHeaders(middlewareDecompression(middlewareCompression(http.FileServerFS(static.Files)))))

	// Start servers
	serverDone := make(chan struct{})
	go func() {
		logger.Sugar().Infof("HTTP server starting on %s", cfg.Server.HttpAddress)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Sugar().Errorf("ListenAndServe http: %v", err)
		}
		close(serverDone)
	}()
	server2Done := make(chan struct{})
	go func() {
		logger.Sugar().Infof("HTTP2 server starting on %s", cfg.Server.HttpsAddress)
		err := server2.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			logger.Sugar().Errorf("ListenAndServe https (http2): %v", err)
		}
		close(server2Done)
	}()

	// Interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Wait for servers to finish
	select {
	case <-interrupt:
		logger.Sugar().Info("Interrupt signal received")
	case <-appCtx.Done():
		logger.Sugar().Info("App stopped")
	case <-serverDone:
		logger.Sugar().Info("HTTP server stopped")
	case <-server2Done:
		logger.Sugar().Info("HTTP2 server stopped")
	}
	logger.Sugar().Info("Server shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(appCtx, 3*time.Second)
	defer cancelShutdown()
	server.Shutdown(shutdownCtx)
	server2.Shutdown(shutdownCtx)
	server.Close()
	server2.Close()
	stopApp()
	db.Close()
	logger.Sugar().Info("Server stopped")
}

// zstdResponseWriter wraps the http.ResponseWriter to provide zstd compression
type zstdResponseWriter struct {
	http.ResponseWriter
	Writer *zstd.Encoder
}

func (w *zstdResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// zstdResponseWriter wraps the io.ReadClose to provide zstd decompression
type zstdRequestReader struct {
	io.ReadCloser
	Reader *zstd.Decoder
}

func (r *zstdRequestReader) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}
