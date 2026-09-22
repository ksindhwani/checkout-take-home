package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/config"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/handlers"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/platform"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/sync/errgroup"
)

type Api struct {
	router          *chi.Mux
	logger          *slog.Logger
	paymentsHandler *handlers.PaymentsHandler
}

func New(cfg config.Config) *Api {
	logger := platform.NewLogger(cfg.LogLevel)

	bankClient := bank.NewHTTPClient(cfg.BankBaseURL, cfg.BankTimeout)
	paymentsRepo := repository.NewPaymentsRepository()
	paymentsService := service.NewPaymentService(paymentsRepo, bankClient)

	a := &Api{
		logger:          logger,
		paymentsHandler: handlers.NewPaymentsHandler(paymentsService),
	}
	a.setupRouter()

	return a
}

func (a *Api) Run(ctx context.Context, addr string) error {
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     a.router,
		BaseContext: func(_ net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-ctx.Done()
		a.logger.Info("shutting down HTTP server")

		// Fresh context: ctx is already cancelled, which would make Shutdown abort immediately.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		a.logger.Info("starting HTTP server", "addr", addr)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return err
		}

		return nil
	})

	return g.Wait()
}

func (a *Api) setupRouter() {
	a.router = chi.NewRouter()
	a.router.Use(middleware.RequestID)
	a.router.Use(middleware.Recoverer)
	a.router.Use(platform.RequestLogging(a.logger))

	a.router.Get("/ping", handlers.PingHandler())
	a.router.Get("/swagger/*", handlers.SwaggerHandler())
	a.router.Handle("/metrics", handlers.MetricsHandler())

	a.router.Route("/api/payments", func(r chi.Router) {
		r.Post("/", a.paymentsHandler.PostHandler())
		r.Get("/{id}", a.paymentsHandler.GetHandler())
	})
}
