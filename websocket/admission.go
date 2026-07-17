package websocket

import (
	"strings"
	"sync"
)

// SubscriptionAdmissionRequest identifies one Client-owned server
// subscription identity. OwnerID is assigned by Client and is opaque to gate
// implementations. The same normalized server identity on two Clients uses
// distinct OwnerIDs and therefore consumes two active-subscription slots.
type SubscriptionAdmissionRequest struct {
	OwnerID        uint64
	ServerIdentity string
	User           string
}

// SubscriptionAdmissionGate atomically admits active server subscription
// identities and their referenced users. Implementations must be safe for
// concurrent use by multiple Clients, return without waiting, and return an
// idempotent, non-blocking release function for every successful acquisition.
type SubscriptionAdmissionGate interface {
	Acquire(SubscriptionAdmissionRequest) (release func(), err error)
}

type subscriptionAdmissionKey struct {
	owner    uint64
	identity string
	user     string
}

type referenceSubscriptionAdmissionGate struct {
	mu               sync.Mutex
	maxSubscriptions int
	maxUsers         int
	references       map[subscriptionAdmissionKey]int
	userReferences   map[string]int
}

// NewSubscriptionAdmissionGate returns a shareable, atomic boundary for the
// official active-subscription and unique-user caps. Each Client owns its wire
// identities independently; user addresses are normalized and shared across
// all Clients using the gate.
func NewSubscriptionAdmissionGate(maxSubscriptions, maxUsers int) SubscriptionAdmissionGate {
	if maxSubscriptions <= 0 {
		maxSubscriptions = DefaultMaxActiveSubscriptions
	}
	if maxUsers <= 0 {
		maxUsers = DefaultMaxUniqueUsers
	}
	return &referenceSubscriptionAdmissionGate{
		maxSubscriptions: maxSubscriptions,
		maxUsers:         maxUsers,
		references:       make(map[subscriptionAdmissionKey]int),
		userReferences:   make(map[string]int),
	}
}

func (g *referenceSubscriptionAdmissionGate) Acquire(request SubscriptionAdmissionRequest) (func(), error) {
	request.User = normalizeSubscriptionUser(request.User)
	key := subscriptionAdmissionKey{owner: request.OwnerID, identity: request.ServerIdentity, user: request.User}
	g.mu.Lock()
	if g.references[key] == 0 {
		if len(g.references) >= g.maxSubscriptions {
			g.mu.Unlock()
			return nil, ErrActiveSubscriptionLimit
		}
		if request.User != "" && g.userReferences[request.User] == 0 && len(g.userReferences) >= g.maxUsers {
			g.mu.Unlock()
			return nil, ErrUniqueUserLimit
		}
		if request.User != "" {
			g.userReferences[request.User]++
		}
	}
	g.references[key]++
	g.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			if g.references[key] > 1 {
				g.references[key]--
				g.mu.Unlock()
				return
			}
			delete(g.references, key)
			if key.user != "" {
				if g.userReferences[key.user] > 1 {
					g.userReferences[key.user]--
				} else {
					delete(g.userReferences, key.user)
				}
			}
			g.mu.Unlock()
		})
	}, nil
}

func normalizeSubscriptionUser(user string) string {
	return strings.ToLower(strings.TrimSpace(user))
}

// admitSubscription is called with c.mu held, after duplicate detection and
// before publishing a new logical subscription in the registry.
func (c *Client) admitSubscription(wire subscriptionWire) error {
	identity := serverSubscriptionIdentity(wire.Subscription)
	if c.subLeases[identity] != nil {
		return nil
	}
	user, _ := wire.Subscription["user"].(string)
	release, err := c.subGate.Acquire(SubscriptionAdmissionRequest{
		OwnerID:        c.subOwner,
		ServerIdentity: identity,
		User:           normalizeSubscriptionUser(user),
	})
	if err != nil {
		return err
	}
	c.subLeases[identity] = release
	return nil
}
