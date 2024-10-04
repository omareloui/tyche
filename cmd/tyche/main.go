package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/dbconn"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	port := config.GetApplicationPort()

	db, q, err := dbconn.New(config.GetDataSource())
	if err != nil {
		log.Fatalln("error connecting to the database", err)
		return
	}
	repo := repository.New(db, q)
	valid := valid.New()
	sani := sanitizer.New()
	h := handler.New(repo, valid, sani)

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router.New(h),
	}

	log.Printf("Starting to listen on http://localhost:%d\n", port)
	log.Fatalln(srv.ListenAndServe())
}
