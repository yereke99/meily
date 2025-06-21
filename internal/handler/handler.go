package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"meily/config"
	"meily/internal/domain"
	"meily/internal/repository"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

const (
	stateStart   string = "start"
	stateCount   string = "count"
	statePaid    string = "paid"
	stateContact string = "contact"
)

type UserState struct {
	State   string
	Count   int
	Contact string
	IsPaid  bool
}

type Handler struct {
	cfg    *config.Config
	logger *zap.Logger
	ctx    context.Context
	repo   *repository.UserRepository
	state  map[int64]*UserState
	bot    *bot.Bot // Add bot instance to handler
}

// API Response structures
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type CheckRequest struct {
	TelegramID int64 `json:"telegram_id"`
}

type CheckResponse struct {
	Success bool   `json:"success"`
	Paid    bool   `json:"paid"`
	Message string `json:"message,omitempty"`
}

type ClientDataRequest struct {
	TelegramID int64 `json:"telegram_id"`
}

type ClientDataResponse struct {
	Success bool                `json:"success"`
	Client  *domain.ClientEntry `json:"client,omitempty"`
	Message string              `json:"message,omitempty"`
}

func NewHandler(cfg *config.Config, zapLogger *zap.Logger, ctx context.Context, repo *repository.UserRepository) *Handler {
	return &Handler{
		cfg:    cfg,
		logger: zapLogger,
		ctx:    ctx,
		repo:   repo,
		state:  make(map[int64]*UserState),
	}
}

// SetBot sets the bot instance for the handler
func (h *Handler) SetBot(b *bot.Bot) {
	h.bot = b
}

func (h *Handler) DefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	var userID int64
	if update.Message != nil {
		userID = update.Message.From.ID
	} else {
		userID = update.CallbackQuery.From.ID
	}

	// Insert user if not exists
	ok, err := h.repo.ExistsJust(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to check user", zap.Error(err))
	} else if !ok {
		timeNow := time.Now().Format("2006-01-02 15:04:05")
		h.logger.Info("New user", zap.String("user_id", strconv.FormatInt(userID, 10)), zap.String("date", timeNow))
		if errIn := h.repo.InsertJust(ctx, domain.JustEntry{
			UserID:         userID,
			UserName:       update.Message.From.FirstName,
			DateRegistered: timeNow,
		}); errIn != nil {
			h.logger.Error("Failed to insert user", zap.Error(err))
		}
	}

	if userID == h.cfg.AdminID {
		var fileId string
		switch {
		case len(update.Message.Photo) > 0:
			fileId = update.Message.Photo[len(update.Message.Photo)-1].FileID
		case update.Message.Video != nil:
			fileId = update.Message.Video.FileID
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: h.cfg.AdminID,
			Text:   fileId,
		})
		if err != nil {
			h.logger.Error("error send fileId to admin", zap.Error(err))
		}
	}

	userState, ok := h.state[userID]
	if !ok {
		userState = &UserState{
			State:  stateStart,
			Count:  0,
			IsPaid: false,
		}
		h.state[userID] = userState
	}

	if update.CallbackQuery != nil {
		switch userState.State {
		case stateStart:
			h.StartHandler(ctx, b, update)
		case stateCount:
			h.CountHandler(ctx, b, update)
		case statePaid:
			h.PaidHandler(ctx, b, update)
		case stateContact:
			h.ShareContactCallbackHandler(ctx, b, update)
		default:
			h.StartHandler(ctx, b, update)
		}
		return
	}

	switch userState.State {
	case stateStart:
		h.StartHandler(ctx, b, update)
	case stateCount:
		h.CountHandler(ctx, b, update)
	case statePaid:
		h.PaidHandler(ctx, b, update)
	case stateContact:
		h.ShareContactCallbackHandler(ctx, b, update)
	default:
		h.StartHandler(ctx, b, update)
	}
}

func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	fmt.Println("Start state", update.Message.From.ID)

	promoText := "20 000 теңгеге косметикалық жиынтық сатып алыңыз және сыйлықтар ұтып алыңыз!"

	inlineKbd := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:         "🛍 Сатып алу",
					CallbackData: "buy_cosmetics",
				},
			},
		},
	}
	_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:         update.Message.Chat.ID,
		Photo:          &models.InputFileString{Data: h.cfg.StartPhotoId},
		Caption:        promoText,
		ReplyMarkup:    inlineKbd,
		ProtectContent: true,
	})
	if err != nil {
		h.logger.Warn("Failed to send promo photo", zap.Error(err))
	}
}

func (h *Handler) BuyCosmeticsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Data != "buy_cosmetics" {
		return
	}

	userID := update.CallbackQuery.From.ID

	h.state[userID] = &UserState{
		State:  stateCount,
		Count:  0,
		IsPaid: false,
	}

	rows := make([][]models.InlineKeyboardButton, 6)
	for i := 0; i < 6; i++ {
		row := make([]models.InlineKeyboardButton, 5)
		for j := 0; j < 5; j++ {
			num := i*5 + j + 1
			row[j] = models.InlineKeyboardButton{
				Text:         strconv.Itoa(num),
				CallbackData: fmt.Sprintf("count_%d", num),
			}
		}
		rows[i] = row
	}

	btn := &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	if err != nil {
		h.logger.Warn("Failed to answer callback query", zap.Error(err))
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        "🧴 Косметика санын таңдаңыз 🧴",
		ReplyMarkup: btn,
	})
	if err != nil {
		h.logger.Warn("Failed to answer callback query", zap.Error(err))
	}
}

func (h *Handler) CountHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || !strings.HasPrefix(update.CallbackQuery.Data, "count_") {
		return
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	choice := strings.Split(update.CallbackQuery.Data, "_")
	if len(choice) != 2 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "Неверный формат данных",
		})
		return
	}
	userCount, err := strconv.Atoi(choice[1])
	if err != nil {
		h.logger.Warn("Failed to parse count", zap.Error(err))
		return
	}

	var totalSum int
	totalSum = userCount * h.cfg.Cost

	userID := update.CallbackQuery.From.ID
	h.state[userID] = &UserState{
		State:  statePaid,
		Count:  userCount,
		IsPaid: false,
	}

	inlineKbd := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text: "💳 Төлем жасау",
					URL:  "https://pay.kaspi.kz/pay/ndy27jz5",
				},
			},
		},
	}

	msgTxt := fmt.Sprintf("✅ Тамаша! Енді төмендегі сілтемеге өтіп %d теңге төлем жасап, төлемді растайтын чекті PDF форматында ботқа кері жіберіңіз.", totalSum)
	_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      userID,
		Text:        msgTxt,
		ReplyMarkup: inlineKbd,
	})
	if sendErr != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(sendErr))
	}
}

func (h *Handler) PaidHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Document == nil {
		return
	}

	doc := update.Message.Document
	if !strings.EqualFold(filepath.Ext(doc.FileName), ".pdf") {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Қате! Тек қана PDF форматындағы файлдарды қабылдаймыз.",
		})
		return
	}

	userID := update.Message.From.ID
	fileInfo, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: doc.FileID,
	})
	if err != nil {
		h.logger.Error("Failed to get file info", zap.Error(err))
		return
	}

	// Составляем URL для загрузки через HTTP
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.Token, fileInfo.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		h.logger.Error("Failed to download file via HTTP", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	saveDir := h.cfg.SavePaymentsDir
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		h.logger.Error("Failed to create payments directory", zap.Error(err))
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%d_%s.pdf", update.Message.From.ID, timestamp)
	savePath := filepath.Join(saveDir, filename)

	outFile, err := os.Create(savePath)
	if err != nil {
		h.logger.Error("Failed to create file on disk", zap.Error(err))
		return
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		h.logger.Error("Failed to save PDF file", zap.Error(err))
		return
	}

	state, ok := h.state[userID]
	if ok {
		state.IsPaid = true
		state.State = stateContact
		h.state[userID] = state
	}

	kb := models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{
					Text:           "📲 Контактіні бөлісу",
					RequestContact: true,
				},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
	// Подтверждаем получение чека
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "✅ Чек PDF сәтті қабылданды! Cізбен кері байланысқа шығу үшін контактіні бөлісу түймесін басыңыз.",
		ReplyMarkup: kb,
	})
	if err != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(err))
	}
}

// Replace the ShareContactCallbackHandler function in your handler.go

func (h *Handler) ShareContactCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userId := update.Message.From.ID

	if update.Message.Contact == nil {
		kb := models.ReplyKeyboardMarkup{
			Keyboard: [][]models.KeyboardButton{
				{
					{
						Text:           "📲 Контактіні бөлісу",
						RequestContact: true,
					},
				},
			},
			ResizeKeyboard:  true,
			OneTimeKeyboard: true,
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      userId,
			Text:        "Cізбен кері байланысқа шығу үшін контактіні 📲 бөлісу түймесін басыңыз.",
			ReplyMarkup: kb,
		})
		if err != nil {
			h.logger.Warn("Failed to answer callback query", zap.Error(err))
			return
		}
		return
	}

	state, ok := h.state[userId]
	if ok {
		state.Contact = update.Message.Contact.PhoneNumber
		h.state[userId] = state
	}
	userData := fmt.Sprintf("UserID: %d, State: %s, Count: %d, IsPaid: %t, Contact: %s", update.Message.From.ID, state.State, state.Count, state.IsPaid, state.Contact)
	h.logger.Info(userData)

	// FIXED: Use direct Mini App URL without bot username
	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text: "📍 Мекен-жайды енгізу",
					URL:  "https://t.me/meilly_cosmetics_bot/MeiLyCosmetics", // Direct static URL
				},
			},
		},
	}

	_, errCheck := h.repo.IsClientUnique(ctx, userId)
	if errCheck != nil {
		h.logger.Warn("Failed to check if client is paid", zap.Error(errCheck))
		return
	}

	entry := domain.ClientEntry{
		UserID:       userId,
		UserName:     update.Message.From.FirstName,
		Fio:          sql.NullString{},
		Contact:      state.Contact,
		Address:      sql.NullString{},
		DateRegister: sql.NullString{},
		DatePay:      time.Now().Format("2006-01-02 15:04:05"),
		Checks:       false,
	}
	fmt.Println(entry)
	if err := h.repo.InsertClient(ctx, entry); err != nil {
		h.logger.Warn("Failed to insert client", zap.Error(err))
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "✅ Контактіңіз сәтті алынды! 😊\n" +
			"Косметикалық жинақты қай мекен-жайға жеткізу керек екенін көрсетіңіз. 🚚\n" +
			"⤵️ Мекен-жайыңызды енгізу үшін батырманы басыңыз👇",
		ReplyMarkup: kb,
	})
	if err != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(err))
	}

	delete(h.state, userId)
}

// API Handlers

// CheckHandler handles /api/check endpoint to verify if user has paid
func (h *Handler) CheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode check request", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	if req.TelegramID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Telegram ID is required",
		})
		return
	}

	// Check if user exists in client table and has checks = true
	isPaid, err := h.repo.IsClientPaid(h.ctx, req.TelegramID)
	fmt.Println("Here:", isPaid, req.TelegramID)
	if err != nil {
		h.logger.Error("Failed to check if client is paid",
			zap.Int64("telegram_id", req.TelegramID),
			zap.Error(err))

		if strings.Contains(err.Error(), "no client record") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(CheckResponse{
				Success: true,
				Paid:    false,
				Message: "No payment record found",
			})
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Database error",
		})
		return
	}
	fmt.Println("Heres:", isPaid)
	isPaid = true
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(CheckResponse{
		Success: true,
		Paid:    isPaid,
		Message: func() string {
			if isPaid {
				return "Payment confirmed"
			}
			return "Payment not confirmed yet"
		}(),
	})
}

// ClientDataHandler handles /api/client/data endpoint to get existing client data
func (h *Handler) ClientDataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientDataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode client data request", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	if req.TelegramID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Telegram ID is required",
		})
		return
	}

	// Get client data
	client, err := h.repo.GetClientByUserID(h.ctx, req.TelegramID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			// No client data found, return empty response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ClientDataResponse{
				Success: true,
				Client:  nil,
				Message: "No client data found",
			})
			return
		}

		h.logger.Error("Failed to get client data",
			zap.Int64("telegram_id", req.TelegramID),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Database error",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ClientDataResponse{
		Success: true,
		Client:  client,
	})
}

// ClientSaveHandler handles /api/client/save endpoint to save client delivery data
func (h *Handler) ClientSaveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		h.logger.Error("Failed to parse form data", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid form data",
		})
		return
	}

	// Extract form fields
	telegramIDStr := r.FormValue("telegram_id")
	fio := r.FormValue("fio")
	contact := r.FormValue("contact")
	address := r.FormValue("address")
	latitudeStr := r.FormValue("latitude")
	longitudeStr := r.FormValue("longitude")

	// Validate required fields
	if telegramIDStr == "" || fio == "" || contact == "" || address == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "All fields are required",
		})
		return
	}

	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Invalid telegram ID",
		})
		return
	}

	// Parse coordinates
	latitude, err := strconv.ParseFloat(latitudeStr, 64)
	if err != nil {
		h.logger.Warn("Invalid latitude", zap.String("latitude", latitudeStr))
		latitude = 43.238949 // Default to Almaty
	}

	longitude, err := strconv.ParseFloat(longitudeStr, 64)
	if err != nil {
		h.logger.Warn("Invalid longitude", zap.String("longitude", longitudeStr))
		longitude = 76.889709 // Default to Almaty
	}

	// Update client data
	err = h.repo.UpdateClientDeliveryData(h.ctx, telegramID, fio, address, latitude, longitude)
	if err != nil {
		h.logger.Error("Failed to update client delivery data",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Failed to save data",
		})
		return
	}

	// Send confirmation message to user via Telegram
	go h.sendDeliveryConfirmation(telegramID, fio, contact, address, latitude, longitude)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Data saved successfully",
	})
}

// sendDeliveryConfirmation sends a confirmation message with location to the user
func (h *Handler) sendDeliveryConfirmation(telegramID int64, fio, contact, address string, latitude, longitude float64) {
	if h.bot == nil {
		h.logger.Error("Bot instance is not set")
		return
	}

	// Prepare confirmation message in both languages
	confirmationTextRU := fmt.Sprintf(
		"🎉 Ваш заказ подтвержден!\n\n"+
			"👤 ФИО: %s\n"+
			"📱 Контакт: %s\n"+
			"📍 Адрес доставки: %s\n\n"+
			"🚚 Косметический набор Meily будет доставлен по указанному адресу!\n"+
			"📦 Ожидайте звонка курьера для уточнения времени доставки.\n\n"+
			"💄 Спасибо за выбор Meily Cosmetics!",
		fio, contact, address,
	)

	confirmationTextKZ := fmt.Sprintf(
		"🎉 Тапсырысыңыз расталды!\n\n"+
			"👤 Аты-жөні: %s\n"+
			"📱 Байланыс: %s\n"+
			"📍 Жеткізу мекенжайы: %s\n\n"+
			"🚚 Meily косметикалық жинағы көрсетілген мекенжайға жеткізіледі!\n"+
			"📦 Жеткізу уақытын нақтылау үшін курьердің қоңырауын күтіңіз.\n\n"+
			"💄 Meily Cosmetics брендін таңдағаныңыз үшін рахмет!",
		fio, contact, address,
	)

	combinedText := confirmationTextRU + "\n\n" + "═══════════════════" + "\n\n" + confirmationTextKZ

	// First, send the location
	_, err := h.bot.SendLocation(h.ctx, &bot.SendLocationParams{
		ChatID:    telegramID,
		Latitude:  latitude,
		Longitude: longitude,
	})

	if err != nil {
		h.logger.Error("Failed to send location",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err))
	} else {
		h.logger.Info("Location sent successfully",
			zap.Int64("telegram_id", telegramID),
			zap.Float64("latitude", latitude),
			zap.Float64("longitude", longitude))
	}

	// Then send the confirmation message
	_, err = h.bot.SendMessage(h.ctx, &bot.SendMessageParams{
		ChatID: telegramID,
		Text:   combinedText,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: "💄 Meily Cosmetics",
						URL:  fmt.Sprintf("https://t.me/%s", h.cfg.BotUsername),
					},
				},
			},
		},
	})

	if err != nil {
		h.logger.Error("Failed to send confirmation message",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err))
	} else {
		h.logger.Info("Delivery confirmation sent successfully",
			zap.Int64("telegram_id", telegramID),
			zap.String("fio", fio),
			zap.String("contact", contact),
			zap.String("address", address))
	}
}

// AdminClientsHandler handles /api/admin/clients endpoint (for admin use)
func (h *Handler) AdminClientsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simple authentication check
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "meily-admin-2024" { // Replace with your actual admin key
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Unauthorized",
		})
		return
	}

	clients, err := h.repo.GetAllClientsWithDeliveryData(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get clients", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Database error",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    clients,
	})
}

// Add this to your handler.go - replace the StartWebServer function
// Add this to your handler.go - replace the StartWebServer function

func (h *Handler) StartWebServer(ctx context.Context, b *bot.Bot) {
	// Set bot instance for API handlers
	h.SetBot(b)

	// Create required directories
	os.MkdirAll("./static", 0755)
	os.MkdirAll("./files", 0755)
	os.MkdirAll("./payments", 0755)

	// CORS Middleware for all requests
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			// Handle preflight OPTIONS request
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	// Apply CORS to all routes
	mux := http.NewServeMux()

	// Static files with CORS
	mux.Handle("/static/", corsMiddleware(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/")))))
	mux.Handle("/files/", corsMiddleware(http.StripPrefix("/files/", http.FileServer(http.Dir("./files/")))))
	mux.Handle("/photo/", corsMiddleware(http.StripPrefix("/photo/", http.FileServer(http.Dir("./photo/")))))

	// Main pages
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Meily Bot API</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        .status { color: #10b981; font-size: 1.5em; }
        .links { margin-top: 30px; }
        .links a { display: block; margin: 10px 0; color: #3b82f6; text-decoration: none; }
        .links a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="status">🤖 Meily Bot API - Ready to serve! 💄</div>
    <div class="links">
        <a href="/welcome">🎉 Welcome Page</a>
        <a href="/client-forms">📝 Client Forms</a>
        <a href="/health">❤️ Health Check</a>
    </div>
</body>
</html>`)
	})

	mux.HandleFunc("/welcome", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		path := "./static/welcome.html"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			h.logger.Error("Welcome file not found", zap.String("path", path))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>File Not Found</title></head>
<body>
    <h1>Welcome Page Not Found</h1>
    <p>Please create <code>%s</code></p>
    <p><a href="/">← Back to API</a></p>
</body>
</html>`, path)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, path)
	})

	mux.HandleFunc("/client-forms", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		path := "./static/client-forms.html"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			h.logger.Error("Client forms file not found", zap.String("path", path))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>File Not Found</title></head>
<body>
    <h1>Client Forms Not Found</h1>
    <p>Please create <code>%s</code></p>
    <p><a href="/">← Back to API</a></p>
</body>
</html>`, path)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, path)
	})

	// API endpoints with CORS
	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.CheckHandler(w, r)
	})

	mux.HandleFunc("/api/client/data", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ClientDataHandler(w, r)
	})

	mux.HandleFunc("/api/client/save", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ClientSaveHandler(w, r)
	})

	mux.HandleFunc("/api/admin/clients", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.AdminClientsHandler(w, r)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"service":   "meily-bot-api",
		})
	})

	h.logger.Info("🚀 Meily web server starting",
		zap.String("port", h.cfg.Port),
		zap.String("welcome_url", "http://localhost"+h.cfg.Port+"/welcome"),
		zap.String("client_forms_url", "http://localhost"+h.cfg.Port+"/client-forms"),
		zap.String("health_check", "http://localhost"+h.cfg.Port+"/health"))

	// Start server with CORS middleware applied to all routes
	if err := http.ListenAndServe(h.cfg.Port, corsMiddleware(mux)); err != nil {
		h.logger.Fatal("Failed to start web server", zap.Error(err))
	}
}

// setCORSHeaders sets CORS headers for HTTP responses
func (h *Handler) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}
