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
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	stateStart      string = "start"
	stateCount      string = "count"
	statePaid       string = "paid"
	stateContact    string = "contact"
	stateAdminPanel string = "admin_panel"
	stateBroadcast  string = "broadcast"
)

type UserState struct {
	State         string
	BroadCastType string
	Count         int
	Contact       string
	IsPaid        bool
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

// Enhanced Admin Dashboard structures
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
	HeatmapData    []map[string]interface{} `json:"heatmapData,omitempty"`
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

func (h *Handler) AdminHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.From.ID != h.cfg.AdminID {
		return
	}

	adminId := h.cfg.AdminID

	h.logger.Info("Admin handler", zap.Any("update", update))

	state, ok := h.state[adminId]
	if ok && state.State == stateBroadcast {
		h.SendMessage(ctx, b, update)
	}

	adminKeyboard := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: "💰 Ақша (Money)"},
				{Text: "👥 Тіркелгендер (Just Clicked)"},
			},
			{
				{Text: "🛍 Клиенттер (Clients)"},
				{Text: "🎲 Лото (Loto)"},
			},
			{
				{Text: "📢 Хабарлама (Messages)"},
				{Text: "🎁 Сыйлық (Gift)"},
			},
			{
				{Text: "📊 Статистика (Statistics)"},
				{Text: "❌ Жабу (Close)"},
			},
		},
		ResizeKeyboard:  true,
		Selective:       true,
		OneTimeKeyboard: true,
	}

	switch update.Message.Text {
	case "/admin":
		h.state[adminId] = &UserState{
			State: stateAdminPanel,
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      adminId,
			Text:        "🔧 Админ панеліне қош келдіңіз!\n\nТаңдаңыз:",
			ReplyMarkup: adminKeyboard,
		})
		if err != nil {
			h.logger.Error("Failed to send admin panel", zap.Error(err))
		}
	case "💰 Ақша (Money)":
		h.handleMoneyStats(ctx, b)

	case "👥 Тіркелгендер (Just Clicked)":
		h.handleJustUsers(ctx, b)

	case "🛍 Клиенттер (Clients)":
		h.handleClients(ctx, b)

	case "🎲 Лото (Loto)":
		h.handleLoto(ctx, b)

	case "📢 Хабарлама (Messages)":
		h.handleBroadcastMenu(ctx, b)

	case "🎁 Сыйлық (Gift)":
		h.handleGift(ctx, b)

	case "📊 Статистика (Statistics)":
		h.handleStatistics(ctx, b)

	case "❌ Жабу (Close)":
		h.handleCloseAdmin(ctx, b)
	default:
		if ok && state.State == stateAdminPanel {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      adminId,
				Text:        "Белгісіз команда. Төмендегі батырмаларды пайдаланыңыз:",
				ReplyMarkup: adminKeyboard,
			})
			if err != nil {
				h.logger.Error("Failed to send admin panel", zap.Error(err))
			}
		}
	}

}

func (h *Handler) SendMessage(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From.ID != h.cfg.AdminID {
		return
	}

	adminId := h.cfg.AdminID
	userState, ok := h.state[adminId]

	switch update.Message.Text {
	case "📢 Барлығына жіберу (All Users)":
		h.startBroadcast(ctx, b, "all")
		return
	case "🛍 Клиенттерге жіберу (Clients Only)":
		h.startBroadcast(ctx, b, "clients")
		return
	case "🎲 Лото қатысушыларына (Loto Participants)":
		h.startBroadcast(ctx, b, "loto")
		return
	case "👥 Тіркелгендерге (Just Users)":
		h.startBroadcast(ctx, b, "just")
		return
	case "🔙 Артқа (Back)":
		delete(h.state, adminId)
		h.AdminHandler(ctx, b, &models.Update{
			Message: &models.Message{
				Text: "/admin",
				From: &models.User{
					ID: adminId,
				},
			},
		})
		return
	}

	if !ok || userState.State != stateBroadcast {
		h.logger.Warn("Admin not in broadcast state", zap.String("current_state", userState.State))
		return
	}

	broadcastType := userState.BroadCastType
	h.logger.Info("Starting broadcast", zap.String("type", broadcastType))

	msgType, fileId, caption := h.parseMessage(update.Message)

	var userIds []int64
	var err error

	switch broadcastType {
	case "all":
		userIds, err = h.repo.GetAllJustUserIDs(ctx)
	case "clients":
		// Assuming you have this method in repository
		userIds, err = h.repo.GetAllJustUserIDs(ctx) // For now, using same as all
	case "loto":
		userIds, err = h.repo.GetAllJustUserIDs(ctx) // For now, using same as all
	case "just":
		userIds, err = h.repo.GetAllJustUserIDs(ctx)
	default:
		err = fmt.Errorf("unknown broadcast type: %s", broadcastType)
	}

	if err != nil {
		h.logger.Error("Failed to load user ids", zap.Error(err))
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminId,
			Text:   fmt.Sprintf("❌ Қате: Пайдаланушы тізімін алу мүмкін болмады\n%s", err.Error()),
		})
		if sendErr != nil {
			h.logger.Error("Failed to send error message", zap.Error(sendErr))
		}
		return
	}

	if len(userIds) == 0 {
		_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminId,
			Text:   "📭 Хабарлама жіберуге пайдаланушылар табылмады",
		})
		if sendErr != nil {
			h.logger.Error("Failed to send no users message", zap.Error(sendErr))
		}
		return
	}

	statusMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminId,
		Text:   fmt.Sprintf("📤 Хабарлама жіберіліп жатыр...\n👥 Жалпы: %d пайдаланушы", len(userIds)),
	})
	if err != nil {
		h.logger.Error("Failed to send status message", zap.Error(err))
		return
	}

	rateLimiter := rate.NewLimiter(rate.Every(time.Second/29), 1)
	var successCount, failedCount int64

	errgroup, ctx := errgroup.WithContext(ctx)
	errgroup.SetLimit(10)

	for i, userId := range userIds {
		usrId := userId
		errgroup.Go(func() error {
			if err := rateLimiter.Wait(ctx); err != nil {
				return err
			}

			if err := h.sendToUser(ctx, b, usrId, msgType, fileId, caption); err != nil {
				atomic.AddInt64(&failedCount, 1)
				h.logger.Warn("Failed to send message to user",
					zap.Int64("user_id", userId),
					zap.Error(err))
				return nil
			} else {
				atomic.AddInt64(&successCount, 1)
			}
			return nil
		})

		if (i+1)%10 == 0 {
			currentSuccess := atomic.LoadInt64(&successCount)
			currentFailed := atomic.LoadInt64(&failedCount)
			progressText := fmt.Sprintf("📤 Хабарлама жіберіліп жатыр...\n👥 Жалпы: %d\n✅ Жіберілді: %d\n❌ Қате: %d\n📊 Прогресс: %.1f%%",
				len(userIds),
				currentSuccess,
				currentFailed,
				float64(currentSuccess+currentFailed)/float64(len(userIds))*100)

			if statusMsg != nil {
				b.EditMessageText(ctx, &bot.EditMessageTextParams{
					ChatID:    adminId,
					MessageID: statusMsg.ID,
					Text:      progressText,
				})
			}
		}
	}

	if err := errgroup.Wait(); err != nil {
		h.logger.Error("Broadcast completed with errors", zap.Error(err))
	}

	// Send final results
	finalSuccess := atomic.LoadInt64(&successCount)
	finalFailed := atomic.LoadInt64(&failedCount)
	successRate := float64(finalSuccess) / float64(len(userIds)) * 100

	finalText := fmt.Sprintf(`✅ ХАБАРЛАМА ЖІБЕРУ АЯҚТАЛДЫ!

👥 Жалпы: %d пайдаланушы
✅ Сәтті: %d
❌ Қате: %d
📊 Сәттілік: %.1f%%

📋 Хабарлама түрі: %s
⏰ Уақыт: %s`,
		len(userIds),
		finalSuccess,
		finalFailed,
		successRate,
		h.getBroadcastTypeName(broadcastType),
		time.Now().Format("2006-01-02 15:04:05"))

	if statusMsg != nil {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    adminId,
			MessageID: statusMsg.ID,
			Text:      finalText,
		})
	}

	// Log broadcast results
	h.logger.Info("Broadcast completed",
		zap.String("type", broadcastType),
		zap.Int("total", len(userIds)),
		zap.Int64("success", finalSuccess),
		zap.Int64("failed", finalFailed),
		zap.Float64("success_rate", successRate))

	delete(h.state, adminId)
	time.Sleep(2 * time.Second)
	h.AdminHandler(ctx, b, &models.Update{
		Message: &models.Message{
			From: &models.User{ID: adminId},
			Text: "/admin",
		},
	})
}

// Helper methods for admin panel
func (h *Handler) handleBroadcastMenu(ctx context.Context, b *bot.Bot) {
	adminId := h.cfg.AdminID

	// Get counts for each category
	allCount, _ := h.repo.GetAllJustUserIDs(ctx)

	broadcastKeyboard := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: fmt.Sprintf("📢 Барлығына жіберу (%d)", len(allCount))},
				{Text: fmt.Sprintf("🛍 Клиенттерге жіберу (%d)", len(allCount))},
			},
			{
				{Text: fmt.Sprintf("🎲 Лото қатысушыларына (%d)", len(allCount))},
				{Text: fmt.Sprintf("👥 Тіркелгендерге (%d)", len(allCount))},
			},
			{
				{Text: "🔙 Артқа (Back)"},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
	}

	message := fmt.Sprintf(`📢 ХАБАРЛАМА ЖІБЕРУ

📊 Қол жетімді аудитория:
• 👥 Барлық пайдаланушылар: %d
• 🛍 Клиенттер: %d  
• 🎲 Лото қатысушылары: %d
• 📅 Тіркелгендер: %d

⚠️ Ескерту: Хабарлама барлық таңдалған пайдаланушыларға жіберіледі. Сақ болыңыз!

Қайсы топқа хабарлама жіберуді қалайсыз?`,
		len(allCount), len(allCount), len(allCount), len(allCount))

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminId,
		Text:        message,
		ReplyMarkup: broadcastKeyboard,
	})
	if err != nil {
		h.logger.Error("Failed to send broadcast menu", zap.Error(err))
	}
}

func (h *Handler) startBroadcast(ctx context.Context, b *bot.Bot, broadcastType string) {
	adminId := h.cfg.AdminID

	// Set admin to broadcast state
	h.state[adminId] = &UserState{
		State:         stateBroadcast,
		BroadCastType: broadcastType,
	}

	targetDescription := h.getBroadcastTypeName(broadcastType)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminId,
		Text: fmt.Sprintf(`📝 ХАБАРЛАМА ЖАЗУ

🎯 Мақсатты аудитория: %s

💡 Қолдаулатын форматтар:
• 📝 Мәтін хабарлама
• 📷 Фото + мәтін
• 🎥 Видео + мәтін  
• 📎 Файл + мәтін
• 🎵 Аудио
• 🎬 GIF анимация

Хабарламаңызды жіберіңіз:`, targetDescription),
		ReplyMarkup: &models.ReplyKeyboardMarkup{
			Keyboard: [][]models.KeyboardButton{
				{{Text: "🔙 Артқа (Back)"}},
			},
			ResizeKeyboard:  true,
			OneTimeKeyboard: false,
		},
	})
	if err != nil {
		h.logger.Error("Failed to start broadcast", zap.Error(err))
	}
}

func (h *Handler) getBroadcastTypeName(broadcastType string) string {
	switch broadcastType {
	case "all":
		return "Барлық пайдаланушылар"
	case "clients":
		return "Барлық клиенттер"
	case "loto":
		return "Лото қатысушылары"
	case "just":
		return "Тіркелген пайдаланушылар"
	default:
		return "Белгісіз"
	}
}

// Placeholder methods - implement these with actual database logic
func (h *Handler) handleMoneyStats(ctx context.Context, b *bot.Bot) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   "💰 АҚША СТАТИСТИКАСЫ\n\n🔧 Дамуда...",
	})
	if err != nil {
		h.logger.Error("Failed to send money stats", zap.Error(err))
	}
}

func (h *Handler) handleJustUsers(ctx context.Context, b *bot.Bot) {
	userIds, err := h.repo.GetAllJustUserIDs(ctx)
	if err != nil {
		h.logger.Error("Failed to get just users", zap.Error(err))
		return
	}

	message := fmt.Sprintf("👥 ТІРКЕЛГЕН ПАЙДАЛАНУШЫЛАР\n\nЖалпы: %d пайдаланушы", len(userIds))
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   message,
	})
	if err != nil {
		h.logger.Error("Failed to send just users", zap.Error(err))
	}
}

func (h *Handler) handleClients(ctx context.Context, b *bot.Bot) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   "🛍 КЛИЕНТТЕР\n\n🔧 Дамуда...",
	})
	if err != nil {
		h.logger.Error("Failed to send clients", zap.Error(err))
	}
}

func (h *Handler) handleLoto(ctx context.Context, b *bot.Bot) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   "🎲 ЛОТО\n\n🔧 Дамуда...",
	})
	if err != nil {
		h.logger.Error("Failed to send loto", zap.Error(err))
	}
}

func (h *Handler) handleGift(ctx context.Context, b *bot.Bot) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   "🎁 СЫЙЛЫҚ\n\n🔧 Дамуда...",
	})
	if err != nil {
		h.logger.Error("Failed to send gift", zap.Error(err))
	}
}

func (h *Handler) handleStatistics(ctx context.Context, b *bot.Bot) {
	userIds, _ := h.repo.GetAllJustUserIDs(ctx)

	message := fmt.Sprintf(`📊 ЖАЛПЫ СТАТИСТИКА

👥 Жалпы пайдаланушылар: %d
🛍 Клиенттер: 0
🎲 Лото қатысушылары: 0

📅 Соңғы жаңарту: %s`,
		len(userIds),
		time.Now().Format("2006-01-02 15:04:05"))

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   message,
	})
	if err != nil {
		h.logger.Error("Failed to send statistics", zap.Error(err))
	}
}

func (h *Handler) handleCloseAdmin(ctx context.Context, b *bot.Bot) {
	delete(h.state, h.cfg.AdminID)

	// Remove keyboard
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: h.cfg.AdminID,
		Text:   "✅ Админ панелі жабылды",
		ReplyMarkup: &models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})
	if err != nil {
		h.logger.Error("Failed to close admin panel", zap.Error(err))
	}
}

// sendToUser отправляет одному пользователю указанное сообщение
func (h *Handler) sendToUser(ctx context.Context, b *bot.Bot, chatID int64, msgType, fileID, caption string) error {
	switch msgType {
	case "text":
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: caption})
		return err
	case "photo":
		_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{ChatID: chatID, Photo: &models.InputFileString{Data: fileID}, Caption: caption})
		return err
	case "video":
		_, err := b.SendVideo(ctx, &bot.SendVideoParams{ChatID: chatID, Video: &models.InputFileString{Data: fileID}, Caption: caption})
		return err
	case "video_note":
		_, err := b.SendVideoNote(ctx, &bot.SendVideoNoteParams{ChatID: chatID, VideoNote: &models.InputFileString{Data: fileID}})
		return err
	case "audio":
		_, err := b.SendAudio(ctx, &bot.SendAudioParams{ChatID: chatID, Audio: &models.InputFileString{Data: fileID}})
		return err
	default:
		return nil
	}
}

func (h *Handler) parseMessage(msg *models.Message) (msgType, fileId, caption string) {
	switch {
	case msg.Text != "":
		return "text", "", msg.Text
	case len(msg.Photo) > 0:
		return "photo", msg.Photo[len(msg.Photo)-1].FileID, msg.Caption
	case msg.Video != nil:
		return "video", msg.Video.FileID, msg.Caption
	case msg.VideoNote != nil:
		return "video_note", msg.VideoNote.FileID, msg.Caption
	case msg.Audio != nil:
		return "audio", msg.Audio.FileID, msg.Caption
	case msg.Location != nil:
		locationStr := fmt.Sprintf("%.6f,%.6f", msg.Location.Latitude, msg.Location.Longitude)
		return "location", "", locationStr
	case msg.Contact != nil:
		contactStr := fmt.Sprintf("%s: %s", msg.Contact.FirstName, msg.Contact.PhoneNumber)
		return "contact", "", contactStr
	default:
		return "", "", ""
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

	// For now, return empty response since we need to implement GetClientByUserID
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ClientDataResponse{
		Success: true,
		Client:  nil,
		Message: "No client data found",
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

	// Save geolocation data
	err = h.repo.InsertGeo(h.ctx, domain.GeoEntry{
		UserID:   telegramID,
		Location: fmt.Sprintf("%.6f,%.6f - %s", latitude, longitude, address),
		DataReg:  time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		h.logger.Error("Failed to save geo data",
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

// Enhanced AdminDashboardHandler with REAL data from clients table - NO MOCK DATA!
func (h *Handler) AdminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	// Get REAL lotto data
	lottoData, err := h.repo.GetRecentLotoEntries(h.ctx, 50)
	if err != nil {
		h.logger.Error("Failed to get recent lotto entries", zap.Error(err))
		lottoData = []domain.LotoEntry{}
	}

	// Get REAL geo data
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

	h.logger.Info("Dashboard data prepared with REAL data from database",
		zap.Int("total_users", totalUsers),
		zap.Int("total_clients", totalClients),
		zap.Int("clients_with_geo", clientsWithGeo),
		zap.Int("total_geo", totalGeo),
		zap.Int("heatmap_points", len(heatmapData)),
		zap.Int("client_data_count", len(clientData)),
		zap.Int("lotto_data_count", len(lottoData)),
		zap.Int("geo_data_count", len(geoData)))

	// Prepare response with REAL data from database
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
