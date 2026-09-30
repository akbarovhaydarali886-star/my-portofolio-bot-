package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"portfolio-bot/api"
	"portfolio-bot/bot"
	"portfolio-bot/config"
	"portfolio-bot/telegram"
)

func main() {
	log.Println("====================================================")
	log.Println("⚡️ Haydarali Akbarov — Portfolio Bot & Backend (Go)")
	log.Println("====================================================")

	// Load configuration
	cfg := config.Load()

	// Initialize Telegram client
	tgClient := telegram.NewClient(cfg.BotToken)

	// Initialize Bot Service
	botService := bot.NewBotService(tgClient, cfg)

	// Initialize Backend HTTP Server
	server := api.NewServer(cfg, botService)

	// Start Telegram bot in background goroutine
	go func() {
		botService.StartBot()
	}()

	// Start HTTP API in background goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Graceful shutdown listener
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	sig := <-stop

	log.Printf("Received signal: %v. Shutting down gracefully...", sig)
}
