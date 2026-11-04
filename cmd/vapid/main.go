// Command vapid prints a new VAPID key pair for the Web Push notifications
// (VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY).
package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%q\nVAPID_PRIVATE_KEY=%q\n", public, private)
}
