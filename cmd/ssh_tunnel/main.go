package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	ssht "github.com/johannessarpola/go-scripts/internal/ssh_tunnel"
	_ "github.com/lib/pq"
)

func main() {
	connAddr := fmt.Sprintf("%s@%s",
		os.Getenv("SSH_USER"),
		os.Getenv("SSH_ADDR"))

	tunnel := ssht.NewSSHTunnel(
		connAddr,
		ssht.NewPrivateKey(os.Getenv("SSH_PRIVATE_KEY_FILE")),
		os.Getenv("SSH_DESTINATION"),
	)

	tunnel.Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)

	go tunnel.Start(context.TODO())
	fmt.Println("Sleeping 100ms")
	time.Sleep(100 * time.Millisecond)

	conn := fmt.Sprintf("host=127.0.0.1 port=%d user=%s password=%s dbname=%s",
		tunnel.Local.Port,
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
	)
	fmt.Println("Opening SQL connection")
	db, err := sql.Open("postgres", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	fmt.Println("Executing SQL")
	r := db.QueryRow("SELECT 1")
	count := -1
	r.Scan(&count)
	fmt.Printf("Should be one: %d\n", count)
}
