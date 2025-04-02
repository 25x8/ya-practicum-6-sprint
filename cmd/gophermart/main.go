package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/25x8/ya-practicum-6-sprint/internal/config"
	"github.com/25x8/ya-practicum-6-sprint/internal/server"
)

func main() {
	cfg := config.NewConfig()

	srv := server.NewServer(cfg)
	go func() {
		if err := srv.Run(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Выключение сервера...")

	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Ошибка при выключении сервера: %v", err)
	}

	log.Println("Сервер успешно выключен")
}
