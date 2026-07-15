package websocket

import "time"

func startHeartbeat(writeJSON func(any) error, config Config) (func(), <-chan error) {
	done := make(chan struct{})
	errors := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := writeJSON(struct {
					Method string `json:"method"`
				}{Method: "ping"}); err != nil {
					select {
					case errors <- err:
					default:
					}
					return
				}
			}
		}
	}()
	return func() { close(done) }, errors
}
