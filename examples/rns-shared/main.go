package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/mytecor/meshbus/core"
	"github.com/mytecor/meshbus/security/realm"
	meshrns "github.com/mytecor/meshbus/transport/rns"
)

func main() {
	realmKey, err := hex.DecodeString(os.Getenv("MESHBUS_REALM_KEY"))
	if err != nil || len(realmKey) != realm.KeySize {
		log.Fatalf("MESHBUS_REALM_KEY must contain %d bytes as hexadecimal", realm.KeySize)
	}
	identityPath := os.Getenv("MESHBUS_IDENTITY")
	if identityPath == "" {
		identityPath = "meshbus.identity"
	}

	node, err := meshrns.NewNode(meshrns.NodeConfig{
		Endpoint: meshrns.Config{
			StackMode:      meshrns.StackSharedClient,
			IdentitySource: identityPath,
			RealmKey:       realmKey,
			PresenceMetadata: map[string]string{
				"service": "rns-shared-example",
			},
		},
		DirectHandler: func(_ context.Context, message core.ReceivedMessage) error {
			fmt.Printf("direct %s: %s\n", message.Sender(), message.Payload())
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer node.Close()

	if _, err := node.Subscribe("example.message", func(_ context.Context, event core.ReceivedEvent) error {
		fmt.Printf("event %s: %s\n", event.Sender, event.Payload)
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := node.Start(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("meshbus node %s started", node.Identity())
	<-ctx.Done()
}
