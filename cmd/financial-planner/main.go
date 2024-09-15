package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/omareloui/financial-planner/internal/config"
	"github.com/omareloui/financial-planner/internal/dbconn"
	"github.com/omareloui/financial-planner/internal/handler"
	"github.com/omareloui/financial-planner/internal/repository"
	"github.com/omareloui/financial-planner/internal/router"
	"github.com/omareloui/financial-planner/internal/sanitizer"
	"github.com/omareloui/financial-planner/internal/valid"
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
