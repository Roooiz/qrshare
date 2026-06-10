package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	qrcode "github.com/skip2/go-qrcode"
)

// getLocalIP returns the first non-loopback IPv4 address found on the machine.
// It is used to build a URL that other devices on the local network can reach.
// Falls back to 127.0.0.1 if no suitable address is found.
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

func main() {
	port := flag.String("p", "8080", "port to serve on")
	dir := flag.String("d", ".", "directory to share")
	flag.Parse()

	// Resolve to absolute path so the file server behaves consistently
	// regardless of how the program was invoked (relative vs absolute path).
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("invalid path: %v", err)
	}

	if err := validateDir(absDir); err != nil {
		log.Fatalf("%v", err)
	}

	ip := getLocalIP()
	url := fmt.Sprintf("http://%s:%s", ip, *port)

	fmt.Printf("📂 Sharing: %s\n", absDir)
	fmt.Printf("🌐 URL:     %s\n\n", url)

	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(qr.ToSmallString(false))

	fs := http.FileServer(http.Dir(absDir))
	http.Handle("/", fs)

	fmt.Println("🚀 Server running. Press Ctrl+C to stop.")
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

// validateDir ensures the given path exists and is a directory.
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
