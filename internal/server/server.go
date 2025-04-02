package server

import (
	"context"
	"net/http"
	"time"

	"github.com/25x8/ya-practicum-6-sprint/internal/config"
	"github.com/25x8/ya-practicum-6-sprint/internal/handlers"
	"github.com/25x8/ya-practicum-6-sprint/internal/middleware"
	"github.com/25x8/ya-practicum-6-sprint/internal/repository"
	"github.com/25x8/ya-practicum-6-sprint/internal/service"
	"github.com/go-chi/chi"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

const (
	secretKey = "some-secret-key"
)

type Server struct {
	cfg            *config.Config
	repo           *repository.PostgresRepository
	accrualSvc     *service.AccrualService
	httpServer     *http.Server
	orderProcessor *service.OrderProcessor
	handlers       *handlers.Handler
}

func NewServer(cfg *config.Config) *Server {
	repo := repository.NewPostgresRepository(cfg.DatabaseURI)
	accrualSvc := service.NewAccrualService(cfg.AccrualSystemAddress)
	orderProcessor := service.NewOrderProcessor(accrualSvc)
	handlers := handlers.NewHandler(repo, accrualSvc, secretKey)

	return &Server{
		cfg:            cfg,
		repo:           repo,
		accrualSvc:     accrualSvc,
		orderProcessor: orderProcessor,
		handlers:       handlers,
	}
}

func (s *Server) Run() error {

	if err := s.repo.InitDB(s.cfg.DatabaseURI); err != nil {
		return err
	}

	s.orderProcessor.Start()

	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", s.handlers.RegisterUser)
		r.Post("/login", s.handlers.LoginUser)

		r.Group(func(r chi.Router) {
			jwtConfig := &middleware.JWTConfig{
				SecretKey: secretKey,
				Repo:      s.repo,
			}
			r.Use(middleware.AuthMiddleware(jwtConfig))

			r.Post("/orders", s.handlers.UploadOrder)
			r.Get("/orders", s.handlers.GetOrders)
			r.Get("/balance", s.handlers.GetBalance)
			r.Post("/balance/withdraw", s.handlers.WithdrawBalance)
			r.Get("/withdrawals", s.handlers.GetWithdrawals)

		})
	})

	s.httpServer = &http.Server{
		Addr:    s.cfg.RunAddress,
		Handler: r,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return err
		}
	}

	if s.orderProcessor != nil {
		s.orderProcessor.Stop()
	}

	if s.repo != nil {
		if err := s.repo.Close(); err != nil {
			return err
		}
	}

	return nil
}
