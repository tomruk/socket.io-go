package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	sio "github.com/tomruk/socket.io-go"
	eio "github.com/tomruk/socket.io-go/engine.io"
)

const addr = "127.0.0.1:3000"

func logEIOServerError(err error) {
	log.Printf("Server error: %v\n", err)
}

func main() {
	io := sio.NewServer(&sio.ServerConfig{
		EIO: eio.ServerConfig{
			OnError: logEIOServerError,
		},
	})

	io.OnConnection(func(socket sio.ServerSocket) {
		fmt.Printf("New socket: %s\n", socket.ID())
		socket.OnEvent("echo", func(message string) {
			fmt.Printf("Message: %s\n", message)
		})
	})

	err := io.Run()
	if err != nil {
		log.Fatalln(err)
	}

	fs := http.FileServer(http.Dir("public"))
	router := http.NewServeMux()

	// Make sure to have a slash at the end of the URL.
	// Otherwise instead of matching with this handler, requests might match with a file that has an socket.io prefix (such as socket.io.min.js).
	router.Handle("/socket.io/", io)

	router.Handle("/", fs)

	fmt.Printf("Listening on: %s\n", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: router,

		// It is always a good practice to set timeouts.
		ReadTimeout: 120 * time.Second,
		IdleTimeout: 120 * time.Second,

		// HTTPWriteTimeout returns io.PollTimeout + 10 seconds (extra 10 seconds to write the response).
		// You should either set this timeout to 0 (infinite) or some value greater than the io.PollTimeout.
		// Otherwise poll requests may fail.
		WriteTimeout: io.HTTPWriteTimeout(),
	}

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalln(err)
	}
}
