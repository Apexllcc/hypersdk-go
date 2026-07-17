package websocket

import "strings"

// admitSubscription is called with c.mu held, after duplicate detection and
// before publishing a new logical subscription in the registry.
func (c *Client) admitSubscription(wire subscriptionWire) error {
	if len(c.subs) >= c.config.MaxActiveSubscriptions {
		return ErrActiveSubscriptionLimit
	}
	user, _ := wire.Subscription["user"].(string)
	user = strings.ToLower(strings.TrimSpace(user))
	if user == "" {
		return nil
	}
	users := make(map[string]struct{})
	for _, subscription := range c.subs {
		existing, _ := subscription.subscriptionWire().Subscription["user"].(string)
		existing = strings.ToLower(strings.TrimSpace(existing))
		if existing != "" {
			users[existing] = struct{}{}
		}
	}
	if _, exists := users[user]; exists {
		return nil
	}
	if len(users) >= c.config.MaxUniqueUsers {
		return ErrUniqueUserLimit
	}
	return nil
}
