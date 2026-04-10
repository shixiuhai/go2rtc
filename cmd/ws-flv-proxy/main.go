package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	var frontend, backend int
	flag.IntVar(&frontend, "l", 8081, "frontend, serve websocket")
	flag.IntVar(&backend, "b", 8080, "backend, which fetch flv stream from")
	flag.Parse()

	fmt.Printf("Transmux http://127.0.0.1:%v/* to ws://:%v/*\n", backend, frontend)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleWS(w, r, backend)
	})

	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%v", frontend), nil); err != nil {
			fmt.Printf("Serve failed, err is %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Println("Press Ctrl+C to exit")
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	fmt.Println("Shutting down...")
}

func handleWS(w http.ResponseWriter, r *http.Request, backend int) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("WebSocket upgrade failed: %v\n", err)
		return
	}
	defer conn.Close()

	url := fmt.Sprintf("http://127.0.0.1:%v%v", backend, r.RequestURI)
	fmt.Printf("Proxy client %s to %s\n", r.RemoteAddr, url)

	client := &http.Client{
		Timeout: 0, // 无限等待，适合长连接
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Connect backend %s failed, err is %v\n", url, err)
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Backend returned status %d\n", resp.StatusCode)
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: HTTP %d", resp.StatusCode)))
		return
	}

	buffer := make([]byte, 8192)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if sendErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); sendErr != nil {
				fmt.Printf("Send to ws failed, err is %v\n", sendErr)
				return
			}
		}

		if err != nil {
			if err == io.EOF {
				fmt.Println("Backend closed connection")
			} else {
				fmt.Printf("Recv from backend failed, err is %v\n", err)
			}
			return
		}
	}
}
