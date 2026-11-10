// Command engine serves the Rackwise game engine over HTTP.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"rackwise/engine/internal/api"
	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/session"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	bots, err := bot.Default()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dictionary loaded: %d words in %d graph nodes", bots.Lex.Count(), bots.Lex.NodeCount())

	store, err := openStore()
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           api.New(bots, session.New(bots, store)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("engine listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}

// openStore uses Postgres when configured. Without it, the engine still runs for
// local development, but games don't survive a restart.
func openStore() (session.Store, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Print("DATABASE_URL is not set: games are kept in memory and lost on restart")
		return session.NewMemoryStore(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return session.OpenPostgres(ctx, url)
}
