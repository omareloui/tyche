package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/omareloui/financial-plans/internal/config"
	"github.com/omareloui/financial-plans/internal/dbconn"
	"github.com/omareloui/financial-plans/internal/router"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	port := config.GetApplicationPort()

	db, err := dbconn.Connect(config.GetDataSource())
	if err != nil {
		log.Fatalln("error connecting to the database", err)
		return
	}

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router.NewRouter(db),
	}

	log.Printf("Starting to listen on http://localhost:%d\n", port)
	log.Fatalln(srv.ListenAndServe())
}
