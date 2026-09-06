// Command genvapid prints a fresh VAPID keypair for Web Push.
//
//	go run ./cmd/genvapid
//
// Set the output as VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY on the server.
package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalf("failed to generate VAPID keys: %v", err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", publicKey)
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", privateKey)
}
