package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
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
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("WebSocket upgrade failed:", err)
			return
		}
		defer conn.Close()

		url := fmt.Sprintf("http://127.0.0.1:%v%v", backend, r.RequestURI)
		fmt.Println("Proxy client", r.RemoteAddr, "to", url)

		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("Connect backend", url, "failed, err is", err)
			return
		}
		defer resp.Body.Close()

		buffer := make([]byte, 1024)
		for {
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				if sendErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); sendErr != nil {
					fmt.Println("Send to ws failed, err is", sendErr)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println("Recv from backend failed, err is", err)
				}
				return
			}
		}
	})

	if err := http.ListenAndServe(fmt.Sprintf(":%v", frontend), nil); err != nil {
		fmt.Println("Serve failed, err is", err)
	}
}
