package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./data/catalog.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
        INSERT INTO products (name, description, category, price_cents, stock) 
        VALUES 
            ('Surfcasting Rod Pro', 'Carbon fiber rod 4.2m', 'rods', 12500, 10),
            ('Surf Lure Pack', '5pk mixed lures', 'lures', 2500, 50)
    `)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("✅ Prodotti inseriti! Ritest grpcurl")
}
