package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/25x8/ya-practicum-6-sprint/internal/middleware"
	"github.com/25x8/ya-practicum-6-sprint/internal/models"
	"github.com/25x8/ya-practicum-6-sprint/internal/repository"
	"github.com/25x8/ya-practicum-6-sprint/internal/service"
	"github.com/25x8/ya-practicum-6-sprint/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Repo       repository.Repository
	AccrualSvc *service.AccrualService
	JWTSecret  string
}

func NewHandler(repo repository.Repository, accrualSvc *service.AccrualService, jwtSecret string) *Handler {
	return &Handler{
		Repo:       repo,
		AccrualSvc: accrualSvc,
		JWTSecret:  jwtSecret,
	}
}

func (h *Handler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	existingUser, err := h.Repo.GetUserByLogin(ctx, req.Login)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if existingUser != nil {
		http.Error(w, "Login already taken", http.StatusConflict)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	userID, err := h.Repo.CreateUser(ctx, req.Login, string(hashedPassword))
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	token, err := middleware.GenerateToken(userID, h.JWTSecret)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	middleware.SetAuthCookie(w, token)
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) LoginUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user, err := h.Repo.GetUserByLogin(ctx, req.Login)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := middleware.GenerateToken(user.ID, h.JWTSecret)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	middleware.SetAuthCookie(w, token)
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	orderNumber := string(body)
	if orderNumber == "" {
		http.Error(w, "Empty order number", http.StatusBadRequest)
		return
	}

	if !utils.IsNumeric(orderNumber) || !utils.ValidateLuhn(orderNumber) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()

	existingOrder, err := h.Repo.GetOrderByNumber(ctx, orderNumber)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if existingOrder != nil && existingOrder.UserID == userID {
		w.WriteHeader(http.StatusOK)
		return
	}

	if existingOrder != nil {
		http.Error(w, "Order already uploaded by another user", http.StatusConflict)
		return
	}

	err = h.Repo.CreateOrder(ctx, userID, orderNumber)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	go h.processOrder(orderNumber)

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	orders, err := h.Repo.GetUserOrders(ctx, userID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type orderResponse struct {
		Number     string    `json:"number"`
		Status     string    `json:"status"`
		Accrual    float64   `json:"accrual,omitempty"`
		UploadedAt time.Time `json:"uploaded_at"`
	}

	response := make([]orderResponse, 0, len(orders))
	for _, order := range orders {
		orderResp := orderResponse{
			Number:     order.Number,
			Status:     order.Status,
			UploadedAt: order.UploadedAt,
		}

		if order.Status == models.StatusProcessed {
			orderResp.Accrual = order.Accrual
		}

		response = append(response, orderResp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	balance, err := h.Repo.GetUserBalance(ctx, userID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

func (h *Handler) WithdrawBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if !utils.IsNumeric(req.Order) || !utils.ValidateLuhn(req.Order) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	err := h.Repo.WithdrawBalance(ctx, userID, req.Order, req.Sum)
	if err != nil {
		if err.Error() == "insufficient funds" {
			http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
			return
		}
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	withdrawals, err := h.Repo.GetUserWithdrawals(ctx, userID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type withdrawalResponse struct {
		Order       string    `json:"order"`
		Sum         float64   `json:"sum"`
		ProcessedAt time.Time `json:"processed_at"`
	}

	response := make([]withdrawalResponse, 0, len(withdrawals))
	for _, w := range withdrawals {
		response = append(response, withdrawalResponse{
			Order:       w.Order,
			Sum:         w.Sum,
			ProcessedAt: w.ProcessedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) processOrder(orderNumber string) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := h.Repo.UpdateOrderStatus(ctx, orderNumber, models.StatusProcessing, 0)
	if err != nil {
		return
	}

	accrualResp, err := h.AccrualSvc.GetOrderAccrual(ctx, orderNumber)
	if err != nil || accrualResp == nil {
		return
	}

	if accrualResp.Status == models.StatusProcessed || accrualResp.Status == models.StatusInvalid {
		h.Repo.UpdateOrderStatus(ctx, orderNumber, accrualResp.Status, accrualResp.Accrual)
	}
}
