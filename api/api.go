package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"portfolio-bot/bot"
	"portfolio-bot/config"
)

type Server struct {
	cfg        *config.Config
	botService *bot.BotService
}

func NewServer(cfg *config.Config, botService *bot.BotService) *Server {
	return &Server{
		cfg:        cfg,
		botService: botService,
	}
}

type OrderRequest struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Telegram   string `json:"telegram,omitempty"`
	Project    string `json:"project"`
	Technology string `json:"technology"`
}

type StandardResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/order", s.handleCreateOrder)

	return withCORS(mux)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/health" {
		http.NotFound(w, r)
		return
	}
	s.handleHealth(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"service":   "portfolio-backend-go",
		"timestamp": time.Now().Format(time.RFC3339),
		"bot_ready": s.cfg.BotToken != "",
		"channel":   s.cfg.ChannelID,
	})
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(StandardResponse{
			Success: false,
			Message: "Noto'g'ri JSON formati",
		})
		return
	}

	if req.Name == "" || req.Phone == "" || req.Project == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(StandardResponse{
			Success: false,
			Message: "Ism, telefon raqam va loyiha ma'lumotlari kiritilishi shart",
		})
		return
	}

	// Format technology label if raw text was sent
	tech := req.Technology
	switch tech {
	case "react", "React", "React.js":
		tech = bot.TechReact
	case "next", "Next", "Next.js":
		tech = bot.TechNext
	case "vue", "Vue", "Vue.js":
		tech = bot.TechVue
	case "":
		tech = "Ko'rsatilmagan"
	}

	session := &bot.UserSession{
		Name:        req.Name,
		Phone:       req.Phone,
		TelegramTag: req.Telegram,
		ProjectInfo: req.Project,
		Technology:  tech,
		UpdatedAt:   time.Now(),
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	s.botService.SendOrderToChannelAndAdmin(session, 0, nowStr, "Portfolio Web-sayti")

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(StandardResponse{
		Success: true,
		Message: "Buyurtma qabul qilindi va Telegram kanaliga yuborildi!",
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) Start() error {
	port := s.cfg.Port
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("🌐 Go Backend API listening on %s (Health check: http://%s/)", addr, addr)
	return http.ListenAndServe(addr, s.SetupRoutes())
}
