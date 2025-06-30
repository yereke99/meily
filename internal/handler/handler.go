package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"meily/config"
	"meily/internal/domain"
	"meily/internal/repository"
	"meily/internal/service"
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
	stateStart      string = "start"
	stateCount      string = "count"
	statePaid       string = "paid"
	stateContact    string = "contact"
	stateAdminPanel string = "admin_panel"
	stateBroadcast  string = "broadcast"
)

type Handler struct {
	cfg       *config.Config
	logger    *zap.Logger
	ctx       context.Context
	repo      *repository.UserRepository
	redisRepo *repository.RedisRepository
	bot       *bot.Bot // Add bot instance to handler
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

// Enhanced Admin Dashboard structures with PROPER coordinate handling
type EnhancedDashboardResponse struct {
	Success        bool                     `json:"success"`
	TotalUsers     int                      `json:"totalUsers"`
	TotalClients   int                      `json:"totalClients"`
	TotalLotto     int                      `json:"totalLotto"`
	TotalGeo       int                      `json:"totalGeo"`
	ClientsWithGeo int                      `json:"clientsWithGeo"`
	LottoStats     *LottoStats              `json:"lottoStats,omitempty"`
	GeoStats       *GeoStats                `json:"geoStats,omitempty"`
	JustData       []domain.JustEntry       `json:"justData,omitempty"`
	ClientData     []ClientEntryWithGeo     `json:"clientData,omitempty"`
	LottoData      []domain.LotoEntry       `json:"lottoData,omitempty"`
	GeoData        []domain.GeoEntry        `json:"geoData,omitempty"`
	OrdersData     []OrderDataForMap        `json:"ordersData,omitempty"` // NEW: Specific for map display
	HeatmapData    []map[string]interface{} `json:"heatmapData,omitempty"`
}

// NEW: Specific structure for map orders display
type OrderDataForMap struct {
	UserID       int64   `json:"userID"`
	UserName     string  `json:"userName"`
	Fio          string  `json:"fio"`
	Contact      string  `json:"contact"`
	Address      string  `json:"address"`
	DateRegister string  `json:"dateRegister"`
	DatePay      string  `json:"dataPay"`
	Checks       bool    `json:"checks"`
	HasGeo       bool    `json:"hasGeo"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Status       string  `json:"status"`     // "delivered", "pending", "processing"
	StatusIcon   string  `json:"statusIcon"` // "✅", "⏳", "📦"
}

// Local structures to match repository types
type LottoStats struct {
	Paid   int `json:"paid"`
	Unpaid int `json:"unpaid"`
}

type GeoStats struct {
	Almaty    int `json:"almaty"`
	Nursultan int `json:"nursultan"`
	Shymkent  int `json:"shymkent"`
	Karaganda int `json:"karaganda"`
	Others    int `json:"others"`
}

type ClientEntryWithGeo struct {
	UserID       int64   `json:"userID"`
	UserName     string  `json:"userName"`
	Fio          string  `json:"fio"`
	Contact      string  `json:"contact"`
	Address      string  `json:"address"`
	DateRegister string  `json:"dateRegister"`
	DatePay      string  `json:"dataPay"`
	Checks       bool    `json:"checks"`
	HasGeo       bool    `json:"hasGeo"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
}

func NewHandler(cfg *config.Config, zapLogger *zap.Logger, ctx context.Context, repo *repository.UserRepository, redisRepo *repository.RedisRepository) *Handler {
	rand.Seed(time.Now().UnixNano())
	return &Handler{
		cfg:       cfg,
		logger:    zapLogger,
		ctx:       ctx,
		repo:      repo,
		redisRepo: redisRepo,
	}
}

// SetBot sets the bot instance for the handler
func (h *Handler) SetBot(b *bot.Bot) {
	h.bot = b
}

// 7. ADD graceful degradation for Redis failures
func (h *Handler) getOrCreateUserState(ctx context.Context, userID int64) *domain.UserState {
	state, err := h.redisRepo.GetUserState(ctx, userID)
	if err != nil {
		h.logger.Error("Redis error, using fallback state",
			zap.Error(err),
			zap.Int64("user_id", userID))

		// Return a safe default state
		return &domain.UserState{
			State:  stateStart,
			Count:  0,
			IsPaid: false,
		}
	}

	if state == nil {
		state = &domain.UserState{
			State:  stateStart,
			Count:  0,
			IsPaid: false,
		}

		// Try to save, but don't fail if Redis is down
		if err := h.redisRepo.SaveUserState(ctx, userID, state); err != nil {
			h.logger.Warn("Failed to save state to Redis, continuing with in-memory state",
				zap.Error(err))
		}
	}

	return state
}

func (h *Handler) JustPaid(ctx context.Context, b *bot.Bot, update *models.Update) {
	doc := update.Message.Document
	if !strings.EqualFold(filepath.Ext(doc.FileName), ".pdf") {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Қате! Тек қана PDF форматындағы файлдарды қабылдаймыз.",
		})
		return
	}

	userID := update.Message.From.ID
	fileInfo, err := b.GetFile(ctx, &bot.GetFileParams{FileID: doc.FileID})
	if err != nil {
		h.logger.Error("Failed to get file info", zap.Error(err))
		return
	}
	fileUrl := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.Token, fileInfo.FilePath)
	resp, err := http.Get(fileUrl)
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
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fileName := fmt.Sprintf("%d_%s.pdf", userID, timestamp)
	savePath := filepath.Join(saveDir, fileName)

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
	h.logger.Info("PDF file saved", zap.String("path", savePath))

	result, err := service.ReadPDF(savePath)
	if err != nil {
		h.logger.Error("Failed to read PDF file", zap.Error(err))
		return
	}
	fmt.Println(result)
	h.logger.Info("PDF file read", zap.Any("result", result))

	actualPrice, err := service.ParsePrice(result[2])
	if err != nil {
		h.logger.Error("error in parse price", zap.Error(err))
		return
	}
	fmt.Println(actualPrice)
	total := actualPrice / h.cfg.Cost
	totalLoto := total * 3
	tickets := make([]int, 0, totalLoto)

	h.logger.Info("price", zap.Any("actualPrice", actualPrice))
	pdfData := domain.PdfResult{
		Total:       total,
		ActualPrice: actualPrice,
		Bin:         h.cfg.Bin,
		Qr:          result[3],
	}
	if err := service.Validator(h.cfg, pdfData); err != nil {
		h.logger.Error("error in validator", zap.Error(err))
		return
	}

	newState := &domain.UserState{
		State:  stateContact,
		Count:  total,
		IsPaid: true,
	}
	if err := h.redisRepo.SaveUserState(ctx, userID, newState); err != nil {
		h.logger.Error("error in save newState to redis", zap.Error(err))
		return
	}

	for i := 0; i < totalLoto; i++ {
		lotoId := rand.Intn(90000000) + 10000000
		if err := h.repo.InsertLoto(ctx, domain.LotoEntry{
			UserID:  userID,
			LotoID:  lotoId,
			QR:      result[3],
			Receipt: savePath,
			DatePay: time.Now().Format("2006-01-02 15:04:05"),
		}); err != nil {
			h.logger.Error("error in insert loto", zap.Error(err))
			return
		}
		tickets = append(tickets, lotoId)
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
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("🎟️ Сізге берілген %d билеті:\n\n", len(tickets)))
	for i := 0; i < len(tickets); i++ {
		sb.WriteString(fmt.Sprintf("•%08d\n", tickets[i]))
	}
	text := sb.String()
	// Чекті сәтті қабылдады
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "✅ Чек PDF сәтті қабылданды!\nCізбен кері байланысқа шығу үшін төмендегі\n📲 Контактіні бөлісу түймесін 👇 міндетті басыңыз.\n\n" + text,
		ReplyMarkup: kb,
	})
	if err != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(err))
	}
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
		if fileId != "" {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: h.cfg.AdminID,
				Text:   fileId,
			})
			if err != nil {
				h.logger.Error("error send fileId to admin", zap.Error(err))
			}
		}
	}

	userState := h.getOrCreateUserState(ctx, userID)
	if update.Message.Document != nil {
		if userState.State != statePaid && userState.State != stateContact {
			h.logger.Info("Document message", zap.String("user_id", strconv.FormatInt(update.Message.From.ID, 10)))
			h.JustPaid(ctx, b, update)
			return
		}
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
		case stateAdminPanel:
			h.AdminHandler(ctx, b, update)
		case stateBroadcast:
			h.SendMessage(ctx, b, update)
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
	case stateAdminPanel:
		h.AdminHandler(ctx, b, update)
	case stateBroadcast:
		h.SendMessage(ctx, b, update)
	default:
		h.StartHandler(ctx, b, update)
	}
}

func (h *Handler) StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	promoText := "18 900 теңгеге косметикалық жиынтық сатып алыңыз және сыйлықтар ұтып алыңыз!"

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

	newState := &domain.UserState{
		State:  stateCount,
		Count:  0,
		IsPaid: false,
	}
	if err := h.redisRepo.SaveUserState(ctx, userID, newState); err != nil {
		h.logger.Error("Failed to save user state to Redis", zap.Error(err))
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

	totalSum := userCount * h.cfg.Cost

	userID := update.CallbackQuery.From.ID
	newState := &domain.UserState{
		State:  statePaid,
		Count:  userCount,
		IsPaid: false,
	}
	if err := h.redisRepo.SaveUserState(ctx, userID, newState); err != nil {
		h.logger.Warn("Failed to save user state in count handler", zap.Error(err))
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
	h.logger.Info("PDF file saved", zap.String("path", savePath))

	result, err := service.ReadPDF(savePath)
	if err != nil {
		h.logger.Warn("Failed to read PDF file", zap.Error(err))
	}

	state, err := h.redisRepo.GetUserState(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to get user state from Redis", zap.Error(err))
	}

	priceInt, errPdf := service.ParsePrice(result[3])
	pdf := domain.PdfResult{
		Total:       state.Count,
		ActualPrice: priceInt,
		Qr:          result[3],
		Bin:         h.cfg.Bin,
	}
	if errPdf != nil {
		h.logger.Error("Failed to parse price from PDF file", zap.Error(err))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   "Дұрыс емес pdf file, қайталап көріңіз",
		})
	}

	if err := service.Validator(h.cfg, pdf); err != nil {
		h.logger.Error("Failed to validate PDF file", zap.Error(err))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   "Дұрыс емес pdf file, қайталап көріңіз",
		})
	}

	if state != nil {
		state.IsPaid = true
		state.State = stateContact
		if err := h.redisRepo.SaveUserState(ctx, userID, state); err != nil {
			h.logger.Error("Failed to save user state to Redis", zap.Error(err))
		}
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

	state, err := h.redisRepo.GetUserState(ctx, userId)
	if err != nil {
		h.logger.Error("Failed to get user state from Redis", zap.Error(err))
		state = &domain.UserState{
			State:  stateContact,
			Count:  1,
			IsPaid: true,
		}
	}

	if state != nil {
		state.Contact = update.Message.Contact.PhoneNumber
		if err := h.redisRepo.SaveUserState(ctx, userId, state); err != nil {
			h.logger.Error("Failed to save user state to Redis", zap.Error(err))
		}
	}

	// FIX: Use state data safely with nil checks
	userData := fmt.Sprintf("UserID: %d, State: %s, Count: %d, IsPaid: %t, Contact: %s",
		update.Message.From.ID,
		func() string {
			if state != nil {
				return state.State
			}
			return "unknown"
		}(),
		func() int {
			if state != nil {
				return state.Count
			}
			return 0
		}(),
		func() bool {
			if state != nil {
				return state.IsPaid
			}
			return false
		}(),
		func() string {
			if state != nil {
				return state.Contact
			}
			return ""
		}())
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

	_, err = b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: update.Message.Chat.ID,
		Video: &models.InputFileString{
			Data: h.cfg.InstructorVideoId,
		},
		Caption: "✅ Контактіңіз сәтті алынды! 😊\n" +
			"Косметикалық жинақты қай мекен-жайға жеткізу керек екенін көрсетіңіз. 🚚\n" +
			"⤵️ Мекен-жайыңызды енгізу үшін батырманы басыңыз👇\nТолығырақ 📹 видео инструкцияда",
		ReplyMarkup:    kb,
		ProtectContent: true,
	})
	if err != nil {
		h.logger.Warn("Failed to send confirmation message", zap.Error(err))
	}

	if err := h.redisRepo.DeleteUserState(ctx, userId); err != nil {
		h.logger.Error("Failed to delete user state from Redis", zap.Error(err))
	}
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

	// Check if user exists in client table
	exists, err := h.repo.ExistsClient(h.ctx, req.TelegramID)
	if err != nil {
		h.logger.Error("Failed to check if client exists",
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
	json.NewEncoder(w).Encode(CheckResponse{
		Success: true,
		Paid:    exists,
		Message: func() string {
			if exists {
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

	// Get client data by UserID
	client, err := h.repo.GetClientByUserID(h.ctx, req.TelegramID)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ClientDataResponse{
				Success: true,
				Client:  nil,
				Message: "No client data found",
			})
			return
		}
		h.logger.Error("Failed to get client data", zap.Error(err))
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
		Message: "Client data found",
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

	// Save geolocation data with proper coordinates format
	locationString := fmt.Sprintf("%.6f,%.6f", latitude, longitude)
	err = h.repo.InsertGeo(h.ctx, domain.GeoEntry{
		UserID:   telegramID,
		Location: locationString,
		DataReg:  time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		h.logger.Error("Failed to save geo data",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err))
	}

	// Update client data with delivery information
	err = h.repo.UpdateClientDeliveryData(h.ctx, telegramID, fio, address, latitude, longitude)
	if err != nil {
		h.logger.Error("Failed to update client delivery data",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err))
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
						URL:  fmt.Sprintf("https://t.me/%s", "meilly_cosmetics_bot"),
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

// Helper method to get total counts safely
func (h *Handler) getTotalCount(ctx context.Context, countFunc func(context.Context) (int, error)) int {
	count, err := countFunc(ctx)
	if err != nil {
		h.logger.Error("Failed to get count", zap.Error(err))
		return 0
	}
	return count
}

// NEW: Helper function to convert AdminClientEntry to OrderDataForMap
func (h *Handler) convertToOrderDataForMap(adminClients []repository.AdminClientEntry) []OrderDataForMap {
	orders := make([]OrderDataForMap, 0, len(adminClients))

	for _, client := range adminClients {
		// Only include clients with valid geolocation
		if !client.HasGeo || client.Latitude == nil || client.Longitude == nil {
			continue
		}

		// Determine order status
		status := "processing"
		statusIcon := "📦"

		if client.Checks {
			status = "delivered"
			statusIcon = "✅"
		} else if client.DatePay != "" && client.DatePay != "null" {
			status = "pending"
			statusIcon = "⏳"
		}

		order := OrderDataForMap{
			UserID:       client.UserID,
			UserName:     client.UserName,
			Fio:          client.Fio,
			Contact:      client.Contact,
			Address:      client.Address,
			DateRegister: client.DateRegister,
			DatePay:      client.DatePay,
			Checks:       client.Checks,
			HasGeo:       true,
			Latitude:     *client.Latitude,
			Longitude:    *client.Longitude,
			Status:       status,
			StatusIcon:   statusIcon,
		}

		orders = append(orders, order)
	}

	return orders
}

// NEW: Helper function to convert ALL geo entries to OrderDataForMap (including those without client records)
func (h *Handler) convertAllGeoToOrderDataForMap(geoEntries []domain.GeoEntry, clientsMap map[int64]repository.AdminClientEntry) []OrderDataForMap {
	orders := make([]OrderDataForMap, 0, len(geoEntries))

	for _, geo := range geoEntries {
		// Parse coordinates from location string
		lat, lon := h.parseGeoCoordinates(geo.Location)
		if lat == nil || lon == nil {
			continue // Skip invalid coordinates
		}

		// Check if this user is also a client
		var status, statusIcon, fio, contact, address, dateRegister, datePay string
		var checks bool

		if client, exists := clientsMap[geo.UserID]; exists {
			// User is both in geo and client tables
			fio = client.Fio
			contact = client.Contact
			address = client.Address
			dateRegister = client.DateRegister
			datePay = client.DatePay
			checks = client.Checks

			if client.Checks {
				status = "delivered"
				statusIcon = "✅"
			} else if client.DatePay != "" && client.DatePay != "null" {
				status = "pending"
				statusIcon = "⏳"
			} else {
				status = "processing"
				statusIcon = "📦"
			}
		} else {
			// User only in geo table (no client record)
			fio = "Геолокация пайдаланушысы"
			contact = "Белгісіз"
			address = geo.Location
			dateRegister = geo.DataReg
			datePay = ""
			checks = false
			status = "processing"
			statusIcon = "📍"
		}

		// Get username from just table or use default
		userName := fmt.Sprintf("User_%d", geo.UserID)

		order := OrderDataForMap{
			UserID:       geo.UserID,
			UserName:     userName,
			Fio:          fio,
			Contact:      contact,
			Address:      address,
			DateRegister: dateRegister,
			DatePay:      datePay,
			Checks:       checks,
			HasGeo:       true,
			Latitude:     *lat,
			Longitude:    *lon,
			Status:       status,
			StatusIcon:   statusIcon,
		}

		orders = append(orders, order)
	}

	return orders
}

// NEW: Helper function to parse geo coordinates from location string
func (h *Handler) parseGeoCoordinates(location string) (*float64, *float64) {
	if location == "" {
		return nil, nil
	}

	// Try different coordinate formats
	// Format 1: "lat,lon"
	if strings.Contains(location, ",") {
		parts := strings.Split(location, ",")
		if len(parts) >= 2 {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil {
				// Validate coordinate ranges
				if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
					return &lat, &lon
				}
			}
		}
	}

	// Format 2: "latitude: 43.2, longitude:  76.8"
	if strings.Contains(location, "latitude:") && strings.Contains(location, "longitude:") {
		latStart := strings.Index(location, "latitude:") + 9
		lonStart := strings.Index(location, "longitude:") + 10

		latEnd := strings.Index(location[latStart:], ",")
		if latEnd == -1 {
			latEnd = len(location) - latStart
		}

		lonEnd := len(location) - lonStart
		if commaIndex := strings.Index(location[lonStart:], ","); commaIndex != -1 {
			lonEnd = commaIndex
		}

		latStr := strings.TrimSpace(location[latStart : latStart+latEnd])
		lonStr := strings.TrimSpace(location[lonStart : lonStart+lonEnd])

		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 == nil && err2 == nil {
			if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
				return &lat, &lon
			}
		}
	}

	return nil, nil
}

// Enhanced AdminDashboardHandler with COMPREHENSIVE ORDERS DATA for MAP DISPLAY
func (h *Handler) AdminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.logger.Info("🔄 Processing admin dashboard request...")

	// Get REAL total counts from database
	totalUsers := h.repo.GetTotalUsers(h.ctx)
	totalClients := h.repo.GetTotalClients(h.ctx)
	totalLotto := h.repo.GetTotalLotto(h.ctx)
	totalGeo := h.repo.GetTotalGeo(h.ctx)

	// Get clients with geolocation count
	clientsWithGeo, err := h.repo.GetClientsWithGeoCount(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get clients with geo count", zap.Error(err))
		clientsWithGeo = 0
	}

	// Get REAL lotto statistics
	repoLottoStats := h.repo.GetLottoStats(h.ctx)
	lottoStats := &LottoStats{
		Paid:   repoLottoStats.Paid,
		Unpaid: repoLottoStats.Unpaid,
	}

	// Get REAL geo statistics by city
	cityStatsMap, err := h.repo.GetGeoStatsByCity(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get geo stats by city", zap.Error(err))
		cityStatsMap = make(map[string]int)
	}

	geoStats := &GeoStats{
		Almaty:    cityStatsMap["almaty"],
		Nursultan: cityStatsMap["nursultan"],
		Shymkent:  cityStatsMap["shymkent"],
		Karaganda: cityStatsMap["karaganda"],
		Others:    cityStatsMap["others"],
	}

	// Get REAL recent data (last 50 records)
	justData, err := h.repo.GetRecentJustEntries(h.ctx, 50)
	if err != nil {
		h.logger.Error("Failed to get recent just entries", zap.Error(err))
		justData = []domain.JustEntry{}
	}

	// Get REAL client data with geolocation using AdminClientEntry directly
	adminClientData, err := h.repo.GetClientsWithGeo(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get clients with geo", zap.Error(err))
		adminClientData = []repository.AdminClientEntry{}
	}

	// Convert repository.AdminClientEntry to our local ClientEntryWithGeo type
	clientData := make([]ClientEntryWithGeo, len(adminClientData))
	for i, client := range adminClientData {
		clientData[i] = ClientEntryWithGeo{
			UserID:       client.UserID,
			UserName:     client.UserName,
			Fio:          client.Fio,
			Contact:      client.Contact,
			Address:      client.Address,
			DateRegister: client.DateRegister,
			DatePay:      client.DatePay,
			Checks:       client.Checks,
			HasGeo:       client.HasGeo,
			Latitude:     0, // Default
			Longitude:    0, // Default
		}

		// Copy coordinates if available
		if client.Latitude != nil {
			clientData[i].Latitude = *client.Latitude
		}
		if client.Longitude != nil {
			clientData[i].Longitude = *client.Longitude
		}
	}

	// Get ALL geo data for comprehensive map display
	allGeoData, err := h.repo.GetAllGeoEntries(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get all geo entries", zap.Error(err))
		allGeoData = []domain.GeoEntry{}
	}

	// Create a map of client data for quick lookup
	clientsMap := make(map[int64]repository.AdminClientEntry)
	for _, client := range adminClientData {
		clientsMap[client.UserID] = client
	}

	// NEW: Create comprehensive orders data for map display from ALL geo entries
	ordersData := h.convertAllGeoToOrderDataForMap(allGeoData, clientsMap)

	h.logger.Info("📍 COMPREHENSIVE Orders data prepared for map display",
		zap.Int("total_admin_clients", len(adminClientData)),
		zap.Int("total_geo_entries", len(allGeoData)),
		zap.Int("orders_for_map", len(ordersData)),
		zap.Int("clients_with_geo_count", clientsWithGeo))

	// Get REAL lotto data
	lottoData, err := h.repo.GetRecentLotoEntries(h.ctx, 50)
	if err != nil {
		h.logger.Error("Failed to get recent lotto entries", zap.Error(err))
		lottoData = []domain.LotoEntry{}
	}

	// Get REAL geo data (limited for table display)
	geoData, err := h.repo.GetRecentGeoEntries(h.ctx, 50)
	if err != nil {
		h.logger.Error("Failed to get recent geo entries", zap.Error(err))
		geoData = []domain.GeoEntry{}
	}

	// Get REAL heatmap data for deliveries (only delivered orders with checks = 1)
	heatmapData, err := h.repo.GetDeliveryHeatmapData(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get delivery heatmap data", zap.Error(err))
		heatmapData = []map[string]interface{}{}
	}

	h.logger.Info("✅ Dashboard data prepared with COMPREHENSIVE REAL data from database",
		zap.Int("total_users", totalUsers),
		zap.Int("total_clients", totalClients),
		zap.Int("clients_with_geo", clientsWithGeo),
		zap.Int("total_geo", totalGeo),
		zap.Int("orders_for_map", len(ordersData)),
		zap.Int("heatmap_points", len(heatmapData)),
		zap.Int("client_data_count", len(clientData)),
		zap.Int("lotto_data_count", len(lottoData)),
		zap.Int("geo_data_count", len(geoData)))

	// Prepare response with COMPREHENSIVE REAL data from database + ALL ORDERS DATA
	response := EnhancedDashboardResponse{
		Success:        true,
		TotalUsers:     totalUsers,
		TotalClients:   totalClients,
		TotalLotto:     totalLotto,
		TotalGeo:       totalGeo,
		ClientsWithGeo: clientsWithGeo,
		LottoStats:     lottoStats,
		GeoStats:       geoStats,
		JustData:       justData,
		ClientData:     clientData,
		LottoData:      lottoData,
		GeoData:        geoData,
		OrdersData:     ordersData, // COMPREHENSIVE: This includes ALL geo entries!
		HeatmapData:    heatmapData,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// AdminClientsHandler handles /api/admin/clients endpoint (for admin use) - REAL DATA
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

	// Get REAL clients data with geolocation from database
	clients, err := h.repo.GetClientsWithGeo(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get clients with geo", zap.Error(err))
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

// GeoAnalyticsHandler handles /api/admin/geo-analytics endpoint - REAL DATA
func (h *Handler) GeoAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	centerLatStr := r.URL.Query().Get("lat")
	centerLonStr := r.URL.Query().Get("lon")
	radiusStr := r.URL.Query().Get("radius")

	if centerLatStr != "" && centerLonStr != "" && radiusStr != "" {
		// Get REAL clients by radius from database
		centerLat, err := strconv.ParseFloat(centerLatStr, 64)
		if err != nil {
			http.Error(w, "Invalid latitude", http.StatusBadRequest)
			return
		}

		centerLon, err := strconv.ParseFloat(centerLonStr, 64)
		if err != nil {
			http.Error(w, "Invalid longitude", http.StatusBadRequest)
			return
		}

		radius, err := strconv.Atoi(radiusStr)
		if err != nil {
			http.Error(w, "Invalid radius", http.StatusBadRequest)
			return
		}

		clients, err := h.repo.GetClientsByLocationRadius(h.ctx, centerLat, centerLon, radius)
		if err != nil {
			h.logger.Error("Failed to get clients by radius", zap.Error(err))
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
		return
	}

	// Default: return REAL heatmap data for delivered orders
	heatmapData, err := h.repo.GetDeliveryHeatmapData(h.ctx)
	if err != nil {
		h.logger.Error("Failed to get delivery heatmap data", zap.Error(err))
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
		Data:    heatmapData,
	})
}

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
        <a href="/admin">👑 Admin Panel</a>
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

	// Admin panel route
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		path := "./static/admin.html"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			h.logger.Error("Admin file not found", zap.String("path", path))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>File Not Found</title></head>
<body>
    <h1>Admin Panel Not Found</h1>
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

	// Enhanced Admin API endpoints
	mux.HandleFunc("/api/admin/dashboard", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.AdminDashboardHandler(w, r)
	})

	mux.HandleFunc("/api/admin/clients", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.AdminClientsHandler(w, r)
	})

	mux.HandleFunc("/api/admin/geo-analytics", func(w http.ResponseWriter, r *http.Request) {
		h.setCORSHeaders(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.GeoAnalyticsHandler(w, r)
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
			"version":   "2.0.0-enhanced",
		})
	})

	h.logger.Info("🚀 Enhanced Meily web server starting",
		zap.String("port", h.cfg.Port),
		zap.String("welcome_url", "http://localhost"+h.cfg.Port+"/welcome"),
		zap.String("client_forms_url", "http://localhost"+h.cfg.Port+"/client-forms"),
		zap.String("admin_url", "http://localhost"+h.cfg.Port+"/admin"),
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
