package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/omareloui/financial-plans/config"
	"github.com/omareloui/financial-plans/internal/routes"
)

func main() {
	port := config.GetApplicationPort()

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: routes.Router(),
	}

	log.Printf("Starting to listen on http://localhost:%d\n", port)
	log.Fatalln(srv.ListenAndServe())
}
