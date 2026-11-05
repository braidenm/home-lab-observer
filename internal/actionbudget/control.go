package actionbudget

import "time"

// ObserveControlTime brackets the entire startup GET+single POST+IPC exchange.
// Five-second deadline equality conservatively refuses; no timer is restarted.
// The authenticated timestamp still comes only from the trusted HTTPS worker.
func (c *Clock) ObserveControlTime(server time.Time, send, recv Stamp) (*Readiness, error) {
	if send.clock != c || recv.clock != c || !continuous(send, recv) || recv.boot-send.boot >= 5*time.Second {
		return nil, ErrUnavailable
	}
	return c.ObserveServerTime(server, send, recv)
}
