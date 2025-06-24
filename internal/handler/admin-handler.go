package handler

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

func (h *Handler) AdminHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message.From.ID != h.cfg.AdminID {
		return
	}

	adminId := h.cfg.AdminID

	h.logger.Info("Admin handler", zap.Any("update", update))

	state, ok := h.state[adminId]
	if ok && state.State == stateBroadcast {
		h.SendMessage(ctx, b, update)
		return
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
	case "📢 Барлығына жіберу":
		h.startBroadcast(ctx, b, "all")
		return
	case "🛍 Клиенттерге жіберу":
		h.startBroadcast(ctx, b, "clients")
		return
	case "🎲 Лото қатысушыларына":
		h.startBroadcast(ctx, b, "loto")
		return
	case "👥 Тіркелгендерге":
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

	h.state[adminId] = &UserState{
		State: stateBroadcast,
	}

	broadcastKeyboard := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: "📢 Барлығына жіберу"},
				{Text: "🛍 Клиенттерге жіберу"},
			},
			{
				{Text: "🎲 Лото қатысушыларына "},
				{Text: "👥 Тіркелгендерге"},
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
	case "document":
		_, err := b.SendDocument(ctx, &bot.SendDocumentParams{ChatID: chatID, Document: &models.InputFileString{Data: fileID}, Caption: caption})
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
	case msg.Document != nil:
		return "document", msg.Document.FileID, msg.Caption
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
