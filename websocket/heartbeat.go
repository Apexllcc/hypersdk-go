package websocket

import (
	"github.com/gorilla/websocket"
	"time"
)

func startHeartbeat(connection *websocket.Conn, config Config) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)) != nil {
					return
				}
			}
		}
	}()
	return func() { close(done) }
}
