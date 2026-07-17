package websocket_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
)

func TestSharedSubscriptionAdmissionGateAcrossClients(t *testing.T) {
	gate := websocket.NewSubscriptionAdmissionGate(1, 1)
	config := websocket.Config{SubscriptionAdmission: gate, ReconnectDelay: time.Hour}
	firstClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = firstClient.Close() }()
	secondClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = secondClient.Close() }()

	first, err := firstClient.SubscribeUserFundings(context.Background(), "0xA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondClient.SubscribeUserFundings(context.Background(), "0xB"); !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
		t.Fatalf("second Client subscription error = %v, want active limit", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := secondClient.SubscribeUserFundings(context.Background(), "0xB")
	if err != nil {
		t.Fatalf("released shared admission was not reusable: %v", err)
	}
	_ = second.Close()
}

func TestSharedSubscriptionAdmissionGateIsAtomicAcrossConcurrentClients(t *testing.T) {
	const limit = 5
	gate := websocket.NewSubscriptionAdmissionGate(limit, 100)
	clients := make([]*websocket.Client, 50)
	for index := range clients {
		clients[index] = websocket.NewClient("ws://example.invalid", websocket.Config{SubscriptionAdmission: gate, ReconnectDelay: time.Hour})
	}
	defer func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}()
	start := make(chan struct{})
	var admitted atomic.Int32
	var group sync.WaitGroup
	for index, client := range clients {
		group.Add(1)
		go func(index int, client *websocket.Client) {
			defer group.Done()
			<-start
			if _, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: fmt.Sprintf("COIN-%d", index)}); err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
				t.Errorf("Client %d admission: %v", index, err)
			}
		}(index, client)
	}
	close(start)
	group.Wait()
	if got := admitted.Load(); got != limit {
		t.Fatalf("concurrently admitted = %d, want %d", got, limit)
	}
}

func TestSharedSubscriptionAdmissionGateCountsSameIdentityPerClientOwner(t *testing.T) {
	gate := websocket.NewSubscriptionAdmissionGate(1, 10)
	config := websocket.Config{SubscriptionAdmission: gate, ReconnectDelay: time.Hour}
	firstClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = firstClient.Close() }()
	secondClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = secondClient.Close() }()
	first, err := firstClient.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if _, err := secondClient.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{}); !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
		t.Fatalf("same identity on a second Client = %v, want active limit", err)
	}
}

func TestSharedSubscriptionAdmissionGateRefcountsServerEquivalentLogicalHandles(t *testing.T) {
	gate := websocket.NewSubscriptionAdmissionGate(1, 10)
	client := websocket.NewClient("ws://example.invalid", websocket.Config{SubscriptionAdmission: gate, ReconnectDelay: time.Hour})
	defer func() { _ = client.Close() }()
	explicitFalse, explicitTrue := false, true
	first, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xA", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xa", IsPortfolioMargin: &explicitTrue})
	if err != nil {
		t.Fatalf("server-equivalent logical handle consumed another lease: %v", err)
	}
	if _, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "OTHER"}); !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
		t.Fatalf("distinct identity while two logical refs live = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "OTHER"}); !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
		t.Fatalf("distinct identity after one logical close = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "OTHER"})
	if err != nil {
		t.Fatalf("last logical close did not release shared server identity: %v", err)
	}
	_ = other.Close()
}

func TestSharedSubscriptionAdmissionGateTracksUsersAcrossClients(t *testing.T) {
	gate := websocket.NewSubscriptionAdmissionGate(10, 1)
	config := websocket.Config{SubscriptionAdmission: gate, ReconnectDelay: time.Hour}
	firstClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = firstClient.Close() }()
	secondClient := websocket.NewClient("ws://example.invalid", config)
	defer func() { _ = secondClient.Close() }()

	first, err := firstClient.SubscribeUserFundings(context.Background(), "0xA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondClient.SubscribeUserFundings(context.Background(), "0xB"); !errors.Is(err, websocket.ErrUniqueUserLimit) {
		t.Fatalf("second Client user error = %v, want unique-user limit", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := secondClient.SubscribeUserFundings(context.Background(), "0xB")
	if err != nil {
		t.Fatalf("released shared user admission was not reusable: %v", err)
	}
	_ = second.Close()
}
