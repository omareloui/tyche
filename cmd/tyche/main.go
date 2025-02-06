package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal("Error loading .env file")
		}
	}
}

func main() {
	port := config.GetApplicationPort()

	repo := repository.New()
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
