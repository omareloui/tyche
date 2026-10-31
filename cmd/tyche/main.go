package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/seeder"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/valid"
)

// initialFxFetchTimeout bounds the blocking fetch on a fresh shared database.
const initialFxFetchTimeout = 2 * time.Minute

func init() {
	err := godotenv.Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal("Error loading .env file")
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	l := logger.Get()
	port := config.GetApplicationPort()

	sharedDB, sharedQ, err := shareddb.Open(config.GetSharedDBPath())
	if err != nil {
		log.Fatalln(err)
	}
	defer sharedDB.Close()

	fxJob := startFxRates(ctx, l, fxrate.NewJob(sharedDB, sharedQ, seeder.CurrencyCodes(), config.GetFxRateFetchInterval()))

	repo := repository.New()
	valid := valid.New()
	sani := sanitizer.New()
	h := handler.New(repo, valid, sani, fxJob)

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router.New(h),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			l.Error("failed to shut down the server", zap.Error(err))
		}
	}()

	log.Printf("Starting to listen on http://localhost:%d\n", port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalln(err)
	}
}

// startFxRates loads the stored rates and runs the job in the background. On
// a fresh database it fetches first, so conversions never start out as 0.
func startFxRates(ctx context.Context, l *zap.Logger, job *fxrate.Job) *fxrate.Job {
	if err := job.Load(ctx); err != nil {
		l.Error("failed to load the fx rates", zap.Error(err))
	}

	if info, err := job.Info(ctx); err == nil && info.Pairs == 0 {
		l.Info("no stored fx rates, fetching them before serving")
		fetchCtx, cancel := context.WithTimeout(ctx, initialFxFetchTimeout)
		if err := job.RunOnce(fetchCtx); err != nil {
			l.Error("failed the initial fx rates fetch", zap.Error(err))
		}
		cancel()
	}

	go job.Run(ctx)
	return job
}
