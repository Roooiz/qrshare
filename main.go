package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	qrcode "github.com/skip2/go-qrcode"
)

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
	port := "8080"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	dir, _ := os.Getwd()
	ip := getLocalIP()
	url := fmt.Sprintf("http://%s:%s", ip, port)

	fmt.Printf("📂 Раздаю: %s\n", dir)
	fmt.Printf("🌐 URL:    %s\n\n", url)

	// QR-код прямо в терминале
        qr, err := qrcode.New(url, qrcode.Medium)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(qr.ToSmallString(false))

	fs := http.FileServer(http.Dir(dir))
	http.Handle("/", fs)

	fmt.Printf("Сервер запущен. Ctrl+C для остановки.\n")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
