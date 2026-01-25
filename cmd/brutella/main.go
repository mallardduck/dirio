package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/brutella/dnssd"
)

var timeFormat = "15:04:05.000"

func main() {
	fmt.Println("Brutella dnssd debugger!")

	instance := "BrutellaTest"
	hostname := "Mac-Studio-BRUTELLA."

	fmt.Printf("Starting dnssd service:\n")
	fmt.Printf("  Instance: %s\n", instance)
	fmt.Printf("  Hostname: %s\n", hostname)
	fmt.Printf("  Service: _http._tcp\n")
	fmt.Printf("  Port: 8000\n")

	// Create responder
	responder, err := dnssd.NewResponder()
	if err != nil {
		fmt.Printf("Error creating responder: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	service, err := dnssd.NewService(dnssd.Config{
		Name:   instance,
		Type:   "_asdf._tcp",
		Domain: "local",
		Host:   hostname,
		Port:   8000,
		Text:   map[string]string{"info": "Brutella test service"},
	})
	if err != nil {
		fmt.Printf("Error creating service: %v\n", err)
		os.Exit(1)
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)

		<-stop
		cancel()
	}()

	// Add service to responder
	go func() {
		time.Sleep(1 * time.Second)
		handle, err := responder.Add(service)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("%s	Got a reply for service %s: Name now registered and active\n", time.Now().Format(timeFormat), handle.Service().ServiceInstanceName())

		<-ctx.Done()
		defer responder.Remove(handle)
	}()
	fmt.Println("Service registered, starting responder....Press Ctrl+C to stop.")
	err = responder.Respond(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("\nShutting down...")
}
