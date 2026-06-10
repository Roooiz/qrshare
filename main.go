package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// generateToken creates a cryptographically secure, URL-safe random string.
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}
	return base64.URLEncoding.EncodeToString(b)
}

// requireAuth middleware checks for a valid session cookie or query token.
// If a valid query token is found, it upgrades it to a cookie for seamless navigation.
func requireAuth(validToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("qrshare_token")
		hasValidCookie := err == nil && cookie.Value == validToken
		hasValidQuery := r.URL.Query().Get("token") == validToken

		if hasValidCookie || hasValidQuery {
			// Upgrade query token to cookie for persistent, clean-URL navigation
			if !hasValidCookie && hasValidQuery {
				http.SetCookie(w, &http.Cookie{
					Name:     "qrshare_token",
					Value:    validToken,
					Path:     "/",
					HttpOnly: true, // Prevents XSS theft
					MaxAge:   86400, // 1 day
				})
				// Redirect to clean URL without the query parameter
				r.URL.RawQuery = ""
				http.Redirect(w, r, r.URL.String(), http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Unauthorized: Please scan the QR code or provide a valid token.", http.StatusUnauthorized)
	})
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func validateDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

func parseMaxSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))

	type multiplier struct {
		suffix string
		mult   int64
	}
	multipliers := []multiplier{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			numStr := strings.TrimSuffix(s, m.suffix)
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return 0, fmt.Errorf("invalid size: %s", s)
			}
			return int64(num * float64(m.mult)), nil
		}
	}

	var num int64
	if _, err := fmt.Sscanf(s, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid size: %s", s)
	}
	return num, nil
}

func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	base = strings.TrimLeft(base, ".")
	if base == "" {
		base = "unnamed_file"
	}
	return base
}

func isSafePath(sharedDir, destPath string) bool {
	absShared, err1 := filepath.Abs(sharedDir)
	absDest, err2 := filepath.Abs(destPath)
	if err1 != nil || err2 != nil {
		return false
	}
	return absDest == absShared || strings.HasPrefix(absDest, absShared+string(os.PathSeparator))
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// rootHandler no longer needs to inject tokens into HTML, the browser handles the cookie.
func rootHandler(maxSize int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Redirect(w, r, "/files/", http.StatusSeeOther)
			return
		}

		dashboardHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>qrshare</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; color: #333; line-height: 1.6; }
        h1 { text-align: center; color: #2c3e50; }
        .card { background: #f8f9fa; border: 2px dashed #dee2e6; border-radius: 12px; padding: 30px; text-align: center; margin-bottom: 30px; }
        input[type=file] { margin: 15px 0; width: 100%%; padding: 10px; border: 1px solid #ced4da; border-radius: 6px; background: white; }
        input[type=text] { width: 100%%; padding: 12px; margin: 10px 0; border: 1px solid #ced4da; border-radius: 6px; box-sizing: border-box; font-size: 16px; }
        button { background: #28a745; color: white; padding: 14px 30px; border: none; border-radius: 6px; cursor: pointer; font-size: 18px; width: 100%%; font-weight: bold; transition: background 0.2s; }
        button:hover { background: #218838; }
        .info { color: #6c757d; font-size: 14px; margin-top: 15px; }
        .browse-btn { display: block; text-align: center; background: #007bff; color: white; text-decoration: none; padding: 14px; border-radius: 6px; font-size: 18px; font-weight: bold; transition: background 0.2s; }
        .browse-btn:hover { background: #0056b3; }
    </style>
</head>
<body>
    <h1>📱 qrshare</h1>
    
    <div class="card">
        <h2>📤 Upload File</h2>
        <form method="POST" enctype="multipart/form-data" action="/upload">
            <input type="file" name="file" required>
            <input type="text" name="target" placeholder="Target subfolder (optional, e.g., 'photos')">
            <button type="submit">Upload to Server</button>
            <p class="info">Max file size: %s</p>
        </form>
    </div>

    <a href="/files/" class="browse-btn">📂 Browse & Download Files</a>
</body>
</html>`, formatBytes(maxSize))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashboardHTML)
	}
}

func uploadHandler(sharedDir string, maxSize int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		if r.ContentLength > maxSize {
			http.Error(w, fmt.Sprintf("File too large. Maximum allowed size is %s.", formatBytes(maxSize)), http.StatusRequestEntityTooLarge)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxSize)

		if err := r.ParseMultipartForm(maxSize); err != nil {
			if strings.Contains(err.Error(), "http: request body too large") || 
			   strings.Contains(err.Error(), "missing colon") {
				http.Error(w, fmt.Sprintf("File too large. Maximum allowed size is %s.", formatBytes(maxSize)), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("Invalid form data: %v", err), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Failed to get uploaded file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		safeName := sanitizeFilename(header.Filename)
		targetDir := filepath.Clean(r.FormValue("target"))
		
		if targetDir == "." || targetDir == "/" {
			targetDir = ""
		}

		destPath := filepath.Join(sharedDir, targetDir, safeName)

		if !isSafePath(sharedDir, destPath) {
			log.Printf("⚠️ Blocked path traversal attempt: %s", destPath)
			http.Error(w, "Invalid target directory", http.StatusBadRequest)
			return
		}

		destPath = uniquePath(destPath)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			http.Error(w, "Failed to create target directory", http.StatusInternalServerError)
			return
		}

		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, "Failed to create file on server", http.StatusInternalServerError)
			return
		}
		defer dest.Close()

		if _, err := io.Copy(dest, file); err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		log.Printf("📥 Uploaded: %s → %s", header.Filename, destPath)
		http.Redirect(w, r, "/files/", http.StatusSeeOther)
	}
}

func main() {
	port := flag.String("p", "8080", "port to serve on")
	dir := flag.String("d", ".", "directory to share")
	maxSizeStr := flag.String("max-size", "100MB", "maximum upload size (e.g. 50MB, 1GB)")
	flag.Parse()

	maxSize, err := parseMaxSize(*maxSizeStr)
	if err != nil {
		log.Fatalf("invalid max-size: %v", err)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("invalid path: %v", err)
	}

	if err := validateDir(absDir); err != nil {
		log.Fatalf("%v", err)
	}

	token := generateToken()
	ip := getLocalIP()
	
	// QR code still uses the query parameter for the initial "handshake"
	secureURL := fmt.Sprintf("http://%s:%s/?token=%s", ip, *port, token)

	fmt.Printf("📂 Sharing: %s\n", absDir)
	fmt.Printf("🔐 Secured with session cookie (Zero-Click via QR)\n")
	fmt.Printf("🌐 Dashboard: http://%s:%s/\n", ip, *port)
	fmt.Printf("📂 Files:     http://%s:%s/files/\n\n", ip, *port)

	qr, err := qrcode.New(secureURL, qrcode.Medium)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(qr.ToSmallString(false))

	// Wrap ALL handlers with the smart cookie/query middleware
	http.Handle("/", requireAuth(token, rootHandler(maxSize)))
	http.Handle("/upload", requireAuth(token, uploadHandler(absDir, maxSize)))
	http.Handle("/files/", requireAuth(token, http.StripPrefix("/files/", http.FileServer(http.Dir(absDir)))))

	fmt.Println("🚀 Server running. Press Ctrl+C to stop.")
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
